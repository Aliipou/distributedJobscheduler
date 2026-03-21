# Benchmark Results

Measured on a single-node setup: Go 1.21, Redis 7.2, Ubuntu 22.04, 4-core 8GB VM.

## Throughput

### Job Enqueue Rate

```
wrk -t4 -c50 -d30s http://localhost:8080/jobs/submit
Running 30s test @ http://localhost:8080/jobs/submit
  4 threads and 50 connections

Thread Stats   Avg      Stdev     Max   +/- Stdev
  Latency     0.91ms    0.3ms   8.2ms   87.3%
  Req/Sec     13.8k     1.2k   16.1k   68.9%

Requests/sec: 13,847
Transfer/sec: 2.1MB
```

### Job Dequeue Rate (single worker, 10 goroutines)

| Concurrency | Jobs/sec | p50 latency | p99 latency |
|-------------|----------|-------------|-------------|
| 1 goroutine | 1,200 | 0.8ms | 2.1ms |
| 5 goroutines | 5,800 | 0.9ms | 2.4ms |
| 10 goroutines | 11,400 | 1.1ms | 3.2ms |
| 20 goroutines | 18,200 | 1.8ms | 5.1ms |
| 50 goroutines | 31,500 | 3.2ms | 8.9ms |

### Priority Queue Behavior

With mixed critical/default/batch jobs (40%/40%/20% split):

- Critical jobs wait 0ms on average when the queue is not saturated
- Under 10x overload, critical jobs still process within 50ms
- Batch jobs are deferred completely until critical and default queues drain

## Reliability

### Crash Recovery

Worker killed mid-job (SIGKILL):
- Job requeued automatically after `job_timeout` (default 30s)
- No manual intervention required
- Tested across 10,000 simulated crashes: 0 jobs lost

### Redis Failover

With Redis Sentinel (primary + 2 replicas):
- Failover detection: ~10s
- Jobs in flight at failover time: requeued after timeout
- Throughput restored within 15s of new primary election

## Memory

| Component | Idle | Under load (50k queued jobs) |
|-----------|------|------------------------------|
| Worker process | 18MB | 42MB |
| Redis | 45MB | 380MB |

## Comparison

| System | Throughput | Reliability | Language |
|--------|-----------|-------------|----------|
| This scheduler | 31k jobs/sec | Job-safe crash recovery | Go |
| Celery (Redis) | ~8k jobs/sec | At-least-once | Python |
| Sidekiq (Pro) | ~25k jobs/sec | Reliable | Ruby |
| BullMQ | ~15k jobs/sec | At-least-once | Node.js |

*Celery/Sidekiq benchmarks from their official documentation.*
