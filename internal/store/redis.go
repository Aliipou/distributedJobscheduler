package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/models"
	"github.com/redis/go-redis/v9"
)

const (
	jobQueueKey    = "scheduler:queue:jobs"
	deadLetterKey  = "scheduler:queue:dead"
	workerPrefix   = "scheduler:worker:"
	lockPrefix     = "scheduler:lock:job:"
	lockTTL        = 30 * time.Second
	workerTTL      = 15 * time.Second
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &RedisStore{client: client}, nil
}

func (r *RedisStore) Close() error {
	return r.client.Close()
}

// AcquireJobLock tries to acquire a distributed lock for scheduling a job.
// Returns true if the lock was acquired.
func (r *RedisStore) AcquireJobLock(ctx context.Context, jobID string) (bool, error) {
	key := lockPrefix + jobID
	ok, err := r.client.SetNX(ctx, key, "1", lockTTL).Result()
	return ok, err
}

// ReleaseJobLock releases the distributed lock for a job.
func (r *RedisStore) ReleaseJobLock(ctx context.Context, jobID string) error {
	return r.client.Del(ctx, lockPrefix+jobID).Err()
}

// EnqueueJob pushes a job to the Redis queue.
func (r *RedisStore) EnqueueJob(ctx context.Context, job *models.QueuedJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return r.client.LPush(ctx, jobQueueKey, data).Err()
}

// DequeueJob blocks until a job is available or timeout occurs.
func (r *RedisStore) DequeueJob(ctx context.Context, timeout time.Duration) (*models.QueuedJob, error) {
	result, err := r.client.BRPop(ctx, timeout, jobQueueKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var job models.QueuedJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// EnqueueDeadLetter moves a failed job to the dead letter queue.
func (r *RedisStore) EnqueueDeadLetter(ctx context.Context, job *models.QueuedJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return r.client.LPush(ctx, deadLetterKey, data).Err()
}

// RegisterWorker registers a worker's heartbeat.
func (r *RedisStore) RegisterWorker(ctx context.Context, info *models.WorkerInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, workerPrefix+info.ID, data, workerTTL).Err()
}

// GetWorkers returns all active workers.
func (r *RedisStore) GetWorkers(ctx context.Context) ([]*models.WorkerInfo, error) {
	keys, err := r.client.Keys(ctx, workerPrefix+"*").Result()
	if err != nil {
		return nil, err
	}
	var workers []*models.WorkerInfo
	for _, key := range keys {
		val, err := r.client.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var w models.WorkerInfo
		if err := json.Unmarshal([]byte(val), &w); err == nil {
			workers = append(workers, &w)
		}
	}
	return workers, nil
}

// QueueLength returns the number of jobs waiting in the queue.
func (r *RedisStore) QueueLength(ctx context.Context) (int64, error) {
	return r.client.LLen(ctx, jobQueueKey).Result()
}
