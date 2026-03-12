package worker

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync/atomic"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/models"
	"github.com/aliipou/distributed-job-scheduler/internal/store"
	"go.uber.org/zap"
)

type Worker struct {
	id          string
	hostname    string
	pg          *store.PostgresStore
	redis       *store.RedisStore
	log         *zap.Logger
	jobsHandled atomic.Int64
}

func New(id string, pg *store.PostgresStore, r *store.RedisStore, log *zap.Logger) *Worker {
	hostname, _ := os.Hostname()
	return &Worker{
		id:       id,
		hostname: hostname,
		pg:       pg,
		redis:    r,
		log:      log,
	}
}

// Run starts the worker loop until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.log.Info("worker started", zap.String("worker_id", w.id))

	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				w.sendHeartbeat(ctx)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker stopping", zap.String("worker_id", w.id))
			return
		default:
			w.processNext(ctx)
		}
	}
}

func (w *Worker) processNext(ctx context.Context) {
	job, err := w.redis.DequeueJob(ctx, 3*time.Second)
	if err != nil {
		if ctx.Err() == nil {
			w.log.Error("dequeue job", zap.Error(err))
		}
		return
	}
	if job == nil {
		return
	}

	w.handle(ctx, job)
}

func (w *Worker) handle(ctx context.Context, job *models.QueuedJob) {
	w.log.Info("executing job",
		zap.String("job_id", job.JobID),
		zap.String("exec_id", job.ExecID),
		zap.Int("attempt", job.Attempt),
	)

	exec, err := w.pg.CreateExecution(ctx, job.JobID, w.id, job.Attempt)
	if err != nil {
		w.log.Error("create execution record", zap.Error(err))
		return
	}

	_ = w.pg.UpdateExecutionStatus(ctx, exec.ID, models.ExecStatusRunning)

	output, execErr := executeCommand(ctx, job.Command, job.Payload, job.TimeoutSec)

	if execErr != nil {
		w.log.Warn("job failed",
			zap.String("job_id", job.JobID),
			zap.Int("attempt", job.Attempt),
			zap.Error(execErr),
		)

		if job.Attempt < job.MaxRetries {
			// Exponential backoff re-queue
			delay := time.Duration(math.Pow(2, float64(job.Attempt-1))) * 10 * time.Second
			time.Sleep(delay)

			job.Attempt++
			if err := w.redis.EnqueueJob(ctx, job); err != nil {
				w.log.Error("re-enqueue job", zap.Error(err))
			}
			_ = w.pg.UpdateExecution(ctx, exec.ID, models.ExecStatusFailed, output, execErr.Error())
		} else {
			w.log.Error("job exhausted retries, moving to dead letter",
				zap.String("job_id", job.JobID),
				zap.Int("attempts", job.Attempt),
			)
			_ = w.redis.EnqueueDeadLetter(ctx, job)
			_ = w.pg.UpdateExecution(ctx, exec.ID, models.ExecStatusDeadLetter, output, execErr.Error())
		}
		return
	}

	w.jobsHandled.Add(1)
	_ = w.pg.UpdateExecution(ctx, exec.ID, models.ExecStatusSuccess, output, "")
	w.log.Info("job succeeded",
		zap.String("job_id", job.JobID),
		zap.String("exec_id", exec.ID),
	)
}

func (w *Worker) sendHeartbeat(ctx context.Context) {
	info := &models.WorkerInfo{
		ID:          w.id,
		Hostname:    w.hostname,
		Status:      "active",
		LastSeen:    time.Now(),
		JobsHandled: w.jobsHandled.Load(),
	}
	if err := w.redis.RegisterWorker(ctx, info); err != nil {
		w.log.Warn("heartbeat failed", zap.Error(err))
	}
}

// Pool manages a group of concurrent workers.
type Pool struct {
	workers []*Worker
}

func NewPool(size int, workerIDPrefix string, pg *store.PostgresStore, r *store.RedisStore, log *zap.Logger) *Pool {
	workers := make([]*Worker, size)
	for i := range workers {
		id := fmt.Sprintf("%s-%d", workerIDPrefix, i+1)
		workers[i] = New(id, pg, r, log)
	}
	return &Pool{workers: workers}
}

func (p *Pool) Run(ctx context.Context) {
	for _, w := range p.workers {
		go w.Run(ctx)
	}
	<-ctx.Done()
}
