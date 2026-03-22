package store

import (
	"context"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/models"
)

// JobStore is the persistence interface used by the HTTP handler and scheduler.
// PostgresStore satisfies this interface.
type JobStore interface {
	CreateJob(ctx context.Context, req *models.CreateJobRequest, nextRun time.Time) (*models.Job, error)
	GetJob(ctx context.Context, id string) (*models.Job, error)
	ListJobs(ctx context.Context, limit, offset int) ([]*models.Job, int64, error)
	UpdateJobStatus(ctx context.Context, id string, status models.JobStatus) error
	UpdateNextRun(ctx context.Context, id string, nextRun time.Time) error
	GetDueJobs(ctx context.Context, now time.Time) ([]*models.Job, error)
	CreateExecution(ctx context.Context, jobID, workerID string, attempt int) (*models.Execution, error)
	UpdateExecution(ctx context.Context, id string, status models.ExecStatus, output, errMsg string) error
	UpdateExecutionStatus(ctx context.Context, id string, status models.ExecStatus) error
	ListExecutions(ctx context.Context, jobID string, limit int) ([]*models.Execution, error)
	GetStats(ctx context.Context) (*models.JobStats, error)
}

// QueueStore is the queue interface used by the HTTP handler and scheduler.
// RedisStore satisfies this interface.
type QueueStore interface {
	AcquireJobLock(ctx context.Context, jobID string, ownerID string) (bool, error)
	ReleaseJobLock(ctx context.Context, jobID string, ownerID string) error
	EnqueueJob(ctx context.Context, job *models.QueuedJob) error
	DequeueJob(ctx context.Context, timeout time.Duration) (*models.QueuedJob, error)
	EnqueueDeadLetter(ctx context.Context, job *models.QueuedJob) error
	RegisterWorker(ctx context.Context, info *models.WorkerInfo) error
	GetWorkers(ctx context.Context) ([]*models.WorkerInfo, error)
	QueueLength(ctx context.Context) (int64, error)
}
