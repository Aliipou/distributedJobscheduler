package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/metrics"
	"github.com/aliipou/distributed-job-scheduler/internal/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// jobDB is the subset of PostgresStore methods used by the scheduler.
type jobDB interface {
	GetDueJobs(ctx context.Context, now time.Time) ([]*models.Job, error)
	UpdateNextRun(ctx context.Context, id string, nextRun time.Time) error
}

// jobQueue is the subset of RedisStore methods used by the scheduler.
type jobQueue interface {
	AcquireJobLock(ctx context.Context, jobID string, ownerID string) (bool, error)
	ReleaseJobLock(ctx context.Context, jobID string, ownerID string) error
	EnqueueJobAtomic(ctx context.Context, job *models.QueuedJob, nextRun time.Time) error
	HasPendingNextRun(ctx context.Context, jobID string) (bool, error)
	ClearPendingNextRun(ctx context.Context, jobID string) error
}

type Scheduler struct {
	pg       jobDB
	redis    jobQueue
	interval time.Duration
	log      *zap.Logger
}

func New(pg jobDB, r jobQueue, intervalSec int, log *zap.Logger) *Scheduler {
	return &Scheduler{
		pg:       pg,
		redis:    r,
		interval: time.Duration(intervalSec) * time.Second,
		log:      log,
	}
}

// Run starts the scheduling loop until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.log.Info("scheduler started", zap.Duration("interval", s.interval))

	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler stopped")
			return
		case t := <-ticker.C:
			s.tick(ctx, t)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	jobs, err := s.pg.GetDueJobs(ctx, now)
	if err != nil {
		s.log.Error("get due jobs", zap.Error(err))
		return
	}

	for _, job := range jobs {
		if err := s.scheduleJob(ctx, job, now); err != nil {
			s.log.Error("schedule job", zap.String("job_id", job.ID), zap.Error(err))
			metrics.ScheduleErrors.WithLabelValues(job.Name).Inc()
		}
	}
}

func (s *Scheduler) scheduleJob(ctx context.Context, job *models.Job, now time.Time) error {
	// Guard against duplicate enqueue caused by a crash between EnqueueJobAtomic
	// and UpdateNextRun: if the atomic marker exists the job is already queued.
	if pending, err := s.redis.HasPendingNextRun(ctx, job.ID); err != nil {
		return err
	} else if pending {
		s.log.Debug("job already pending in queue, skipping re-enqueue",
			zap.String("job_id", job.ID))
		return nil
	}

	// Acquire distributed lock to prevent double-scheduling across scheduler
	// replicas. The ownerID is unique per attempt so only the holder can
	// release the lock (see the Lua script in RedisStore.ReleaseJobLock).
	ownerID := uuid.New().String()
	acquired, err := s.redis.AcquireJobLock(ctx, job.ID, ownerID)
	if err != nil {
		return err
	}
	if !acquired {
		s.log.Debug("job already locked, skipping", zap.String("job_id", job.ID))
		return nil
	}
	defer func() { _ = s.redis.ReleaseJobLock(ctx, job.ID, ownerID) }()

	// Compute next run time
	nextRun, err := NextRun(job.CronExpr, now)
	if err != nil {
		return err
	}

	// Create execution record
	execID := uuid.New().String()
	payload, _ := json.Marshal(job.Payload)

	queued := &models.QueuedJob{
		JobID:      job.ID,
		ExecID:     execID,
		Command:    job.Command,
		Payload:    string(payload),
		Attempt:    1,
		MaxRetries: job.MaxRetries,
		TimeoutSec: job.TimeoutSeconds,
	}

	// Atomically push the job to Redis AND record the pending next-run marker.
	// If the process crashes after this line, HasPendingNextRun will prevent
	// re-enqueue on the next scheduler tick.
	if err := s.redis.EnqueueJobAtomic(ctx, queued, nextRun); err != nil {
		return err
	}

	// Persist the new next_run_at in Postgres. If this fails the pending marker
	// in Redis prevents a duplicate execution; the marker TTL (5 min) acts as a
	// circuit-breaker so the job eventually becomes schedulable again.
	if err := s.pg.UpdateNextRun(ctx, job.ID, nextRun); err != nil {
		return err
	}

	// Postgres confirmed: remove the Redis pending marker.
	if err := s.redis.ClearPendingNextRun(ctx, job.ID); err != nil {
		// Non-fatal: the marker will expire on its own.
		s.log.Warn("clear pending next-run marker",
			zap.String("job_id", job.ID), zap.Error(err))
	}

	metrics.JobsEnqueued.WithLabelValues(job.Name).Inc()
	s.log.Info("job enqueued",
		zap.String("job_id", job.ID),
		zap.String("job_name", job.Name),
		zap.Time("next_run", nextRun),
	)
	return nil
}
