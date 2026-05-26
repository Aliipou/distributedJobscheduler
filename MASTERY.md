# Mastery Engineering Audit — Distributed Job Scheduler

> "Simplicity under pressure = mastery"

## Project Summary
A distributed, fault-tolerant job scheduler using Go, Redis (queue + distributed locking), and PostgreSQL (persistence). Workers are stateless; the scheduler loop polls for due cron jobs, acquires distributed locks, and enqueues work to a Redis FIFO queue.

## Failure Mode Analysis

### What breaks when the main service goes down?
The REST API and scheduler loop share the same binary. In-flight jobs already dequeued by workers continue executing normally. Jobs due during the downtime are **silently skipped** — there is no catch-up logic after restart. `GetDueJobs` only returns jobs where `next_run_at <= now`, so the missed window is lost. The next tick schedules the *next* cron occurrence, not the missed one.

### What breaks when the database/storage slows down?
The scheduler tick calls `GetDueJobs` on every interval with no per-tick context timeout. Slow Postgres stalls the entire tick loop. Workers calling `CreateExecution` and `UpdateExecution` on every job also stall; the `pgxpool` connection pool will exhaust under sustained latency, causing dequeued jobs to be processed but execution records to fail — status is logged as a warning and the job is silently dropped from tracking.

### What breaks when the network is partitioned?
If Redis is unreachable, `AcquireJobLock` fails, jobs are not enqueued, the error is logged, and the scheduler continues looping — a safe degradation. Workers block on `BRPop` with a 3-second timeout and retry. If Postgres is partitioned but Redis is alive, jobs get enqueued but `CreateExecution` fails — the worker logs the error and **abandons the dequeued message**, which is now silently lost (it was already consumed from the Redis queue).

### What breaks under duplicate execution?
The scheduler uses `SetNX` + Lua CAS release — the correct pattern. However, `UpdateNextRun` is called *after* `EnqueueJob` without a transaction. A crash between these two calls leaves the old `next_run_at` intact, causing the same job to be re-enqueued on the very next scheduler tick — **genuine duplicate execution**.

## Checklist Status

| Check | Status | Notes |
|-------|--------|-------|
| Invariants defined | ⚠️ | Job/execution status transitions documented in README but no DB-level CHECK constraints enforce valid values |
| Idempotency | ⚠️ | Enqueue + UpdateNextRun are not atomic; a crash between them causes duplicate enqueue on next tick |
| Race conditions handled | ✅ | Distributed lock uses SetNX + Lua CAS release — correct implementation with owner-ID guard |
| State consistency | ⚠️ | EnqueueJob then UpdateNextRun is two separate operations; partial write leaves inconsistent scheduler state |
| Structured logging | ✅ | zap used throughout with job_id, exec_id, worker_id, attempt on every relevant log call |
| Metrics (not just logs) | ❌ | No Prometheus /metrics endpoint; only an internal atomic jobsHandled counter — nothing is scraped externally |
| Distributed tracing | ❌ | No OpenTelemetry instrumentation; cannot correlate scheduler tick → enqueue → worker → execution as a single trace |
| Rollback strategy | ⚠️ | Single 001_init.sql migration with no down-migration; rollback requires manual intervention |
| Safe migrations | ⚠️ | Migrate() re-runs raw SQL on every startup; safe only because of IF NOT EXISTS — no migration version table |
| 10x traffic plan | ⚠️ | Workers scale horizontally (stateless), but the single scheduler loop is a serialization bottleneck at high job counts |
| Bottleneck identified | ⚠️ | Single scheduler Postgres polling loop; Redis LPUSH/BRPOP is O(1) and not the bottleneck |
| Simplicity test passed | ✅ | Clean three-component architecture: scheduler → Redis queue → worker pool |

## Critical Gaps (Must Fix)

1. **Enqueue + UpdateNextRun atomicity**: record `next_run_at` in Postgres *before* calling `EnqueueJob`, or wrap both in a transaction with a unique exec_id as an idempotency key. The current ordering guarantees duplicate execution on scheduler crash.
2. **No Prometheus metrics**: the system has zero external observability. At minimum expose `jobs_enqueued_total`, `jobs_processed_total{status}`, `jobs_retried_total`, queue depth (LLEN), and dead-letter queue depth.
3. **Missed-execution gap on restart**: cron jobs due during downtime are silently skipped. Add a startup check that logs (or optionally re-runs) any job whose `next_run_at` is more than one interval in the past.
4. **No DB-level status constraints**: `jobs.status` and `executions.status` are free `VARCHAR(50)` with no `CHECK` constraint — invalid states persist silently.
5. **Abandoned dequeued messages on DB failure**: when `CreateExecution` fails after dequeue, the job is consumed from Redis but never recorded. Consider re-enqueuing on DB error or using a two-phase acknowledge pattern.

## What is Already Mastery-Level

- **Distributed locking**: `SetNX` + Lua CAS release is the correct Redis locking pattern. The owner-ID check prevents a slow worker from releasing a lock re-acquired by another instance.
- **Dead-letter queue**: exhausted-retry jobs move to `scheduler:queue:dead` — they are not silently dropped.
- **Exponential backoff**: `2^(attempt-1) * 10s` delay prevents retry thundering-herd on transient failures.
- **Worker heartbeats with TTL**: workers register with a 15-second TTL so stale entries auto-expire without a cleanup job.
- **Interface-based dependencies**: `jobDB` and `jobQueue` interfaces enable clean unit testing without real infrastructure.
- **SCAN instead of KEYS**: `GetWorkers` uses cursor-based `SCAN` to avoid blocking Redis on large keyspaces.

## Recommended Next Steps

1. **Atomic scheduling**: write `next_run_at` to Postgres *before* enqueueing, keyed on a pre-generated `exec_id`; use a Redis SET as an idempotency guard so a duplicate enqueue within the lock TTL is a no-op.
2. **Add /metrics endpoint**: expose Prometheus counters and gauges for queue depth, jobs/s, retry rate, dead-letter backlog, and scheduler tick latency.
3. **Scheduler HA**: run two scheduler instances — the existing distributed lock already prevents double-scheduling, making this a free reliability win.
4. **Migration versioning**: replace the raw-SQL `Migrate()` call with golang-migrate or a simple version table so incremental schema changes and rollbacks are safe.
5. **OpenTelemetry tracing**: propagate a trace context from `scheduleJob` through the Redis queue (in the `QueuedJob` payload) to `worker.handle` so end-to-end job latency is visible as a single trace.
