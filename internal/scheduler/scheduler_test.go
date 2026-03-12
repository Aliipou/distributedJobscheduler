package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/models"
	"github.com/aliipou/distributed-job-scheduler/internal/scheduler"
	"go.uber.org/zap"
)

// ── Fake stores ───────────────────────────────────────────────────────────────

type fakePG struct {
	mu       sync.Mutex
	jobs     []*models.Job
	nextRuns map[string]time.Time
	dueErr   error
	nextErr  error
}

func (f *fakePG) GetDueJobs(_ context.Context, now time.Time) ([]*models.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dueErr != nil {
		return nil, f.dueErr
	}
	var due []*models.Job
	for _, j := range f.jobs {
		if j.Status == models.JobStatusActive && j.NextRunAt != nil && !j.NextRunAt.After(now) {
			due = append(due, j)
		}
	}
	return due, nil
}

func (f *fakePG) UpdateNextRun(_ context.Context, id string, next time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.nextErr != nil {
		return f.nextErr
	}
	if f.nextRuns == nil {
		f.nextRuns = make(map[string]time.Time)
	}
	f.nextRuns[id] = next
	return nil
}

type fakeRedis struct {
	mu      sync.Mutex
	queue   []*models.QueuedJob
	lockErr error
	enqErr  error
}

func (f *fakeRedis) AcquireJobLock(_ context.Context, _ string) (bool, error) {
	return true, f.lockErr
}

func (f *fakeRedis) ReleaseJobLock(_ context.Context, _ string) error { return nil }

func (f *fakeRedis) EnqueueJob(_ context.Context, job *models.QueuedJob) error {
	if f.enqErr != nil {
		return f.enqErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, job)
	return nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestScheduler_EnqueuesDueJobs(t *testing.T) {
	past := time.Now().Add(-1 * time.Minute)
	pg := &fakePG{
		jobs: []*models.Job{
			{ID: "job-1", Name: "sync", CronExpr: "* * * * *", Command: "sync.sh",
				Status: models.JobStatusActive, NextRunAt: &past, MaxRetries: 3},
		},
	}
	redis := &fakeRedis{}

	sched := scheduler.New(pg, redis, 1, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run scheduler for a short time
	go sched.Run(ctx)
	time.Sleep(1500 * time.Millisecond)
	cancel()

	redis.mu.Lock()
	queued := len(redis.queue)
	redis.mu.Unlock()

	if queued == 0 {
		t.Error("expected at least one job to be enqueued")
	}
	if redis.queue[0].JobID != "job-1" {
		t.Errorf("expected job-1, got %s", redis.queue[0].JobID)
	}
}

func TestScheduler_SkipsPausedJobs(t *testing.T) {
	past := time.Now().Add(-1 * time.Minute)
	pg := &fakePG{
		jobs: []*models.Job{
			{ID: "job-paused", CronExpr: "* * * * *", Command: "x",
				Status: models.JobStatusPaused, NextRunAt: &past},
		},
	}
	redis := &fakeRedis{}

	sched := scheduler.New(pg, redis, 1, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	sched.Run(ctx)

	if len(redis.queue) != 0 {
		t.Errorf("expected no queued jobs for paused job, got %d", len(redis.queue))
	}
}

func TestScheduler_SkipsFutureJobs(t *testing.T) {
	future := time.Now().Add(10 * time.Minute)
	pg := &fakePG{
		jobs: []*models.Job{
			{ID: "job-future", CronExpr: "* * * * *", Command: "x",
				Status: models.JobStatusActive, NextRunAt: &future},
		},
	}
	redis := &fakeRedis{}

	sched := scheduler.New(pg, redis, 1, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	sched.Run(ctx)

	if len(redis.queue) != 0 {
		t.Errorf("expected no queued jobs for future job, got %d", len(redis.queue))
	}
}

func TestScheduler_UpdatesNextRunAfterEnqueue(t *testing.T) {
	past := time.Now().Add(-1 * time.Minute)
	pg := &fakePG{
		jobs: []*models.Job{
			{ID: "job-1", CronExpr: "* * * * *", Command: "x",
				Status: models.JobStatusActive, NextRunAt: &past, MaxRetries: 3},
		},
	}
	redis := &fakeRedis{}

	sched := scheduler.New(pg, redis, 1, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	sched.Run(ctx)

	pg.mu.Lock()
	nextRun, updated := pg.nextRuns["job-1"]
	pg.mu.Unlock()

	if !updated {
		t.Fatal("expected next_run to be updated")
	}
	if !nextRun.After(time.Now().Add(-time.Second)) {
		t.Errorf("next_run should be in the future, got %v", nextRun)
	}
}

func TestScheduler_HandlesGetDueJobsError(t *testing.T) {
	pg := &fakePG{dueErr: errors.New("pg down")}
	redis := &fakeRedis{}

	sched := scheduler.New(pg, redis, 1, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	// Should not panic even with DB errors
	sched.Run(ctx)
}

func TestScheduler_MultipleJobs(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Second)
	pg := &fakePG{
		jobs: []*models.Job{
			{ID: "j1", CronExpr: "* * * * *", Command: "a", Status: models.JobStatusActive, NextRunAt: &past, MaxRetries: 3},
			{ID: "j2", CronExpr: "* * * * *", Command: "b", Status: models.JobStatusActive, NextRunAt: &past, MaxRetries: 3},
			{ID: "j3", CronExpr: "* * * * *", Command: "c", Status: models.JobStatusActive, NextRunAt: &past, MaxRetries: 3},
		},
	}
	redis := &fakeRedis{}

	sched := scheduler.New(pg, redis, 1, zap.NewNop())
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	sched.Run(ctx)

	redis.mu.Lock()
	queued := len(redis.queue)
	redis.mu.Unlock()

	if queued < 3 {
		t.Errorf("expected at least 3 queued jobs, got %d", queued)
	}
}

func TestScheduler_StopGracefully(t *testing.T) {
	pg := &fakePG{}
	redis := &fakeRedis{}

	sched := scheduler.New(pg, redis, 1, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sched.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Error("scheduler did not stop within 2 seconds")
	}
}
