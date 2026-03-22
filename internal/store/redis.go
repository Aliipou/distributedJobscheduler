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
// The lock value is set to ownerID so that only the lock holder can release it.
// Returns true if the lock was acquired.
func (r *RedisStore) AcquireJobLock(ctx context.Context, jobID string, ownerID string) (bool, error) {
	key := lockPrefix + jobID
	ok, err := r.client.SetNX(ctx, key, ownerID, lockTTL).Result()
	return ok, err
}

// releaseJobLockScript is a Lua script that releases a lock only if the caller
// owns it. This prevents a worker from accidentally releasing a lock that was
// re-acquired by another worker after the original TTL expired.
var releaseJobLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// ReleaseJobLock releases the distributed lock for a job, but only if ownerID
// matches the value that was set during AcquireJobLock. This avoids a race
// condition where a slow worker deletes a lock that a different worker has
// since re-acquired.
func (r *RedisStore) ReleaseJobLock(ctx context.Context, jobID string, ownerID string) error {
	key := lockPrefix + jobID
	_, err := releaseJobLockScript.Run(ctx, r.client, []string{key}, ownerID).Result()
	if err == redis.Nil {
		// Lock was not held by this owner; treat as a no-op.
		return nil
	}
	return err
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
// Uses SCAN instead of KEYS to avoid blocking Redis on large keyspaces.
func (r *RedisStore) GetWorkers(ctx context.Context) ([]*models.WorkerInfo, error) {
	var workers []*models.WorkerInfo
	var cursor uint64
	for {
		keys, next, err := r.client.Scan(ctx, cursor, workerPrefix+"*", 100).Result()
		if err != nil {
			return nil, err
		}
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
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return workers, nil
}

// QueueLength returns the number of jobs waiting in the queue.
func (r *RedisStore) QueueLength(ctx context.Context) (int64, error) {
	return r.client.LLen(ctx, jobQueueKey).Result()
}
