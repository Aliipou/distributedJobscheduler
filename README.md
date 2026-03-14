# Distributed Job Scheduler

> A production-grade distributed job scheduling system in Go — cron-based scheduling, distributed locking, horizontal worker scaling, exponential-backoff retry, dead-letter queues, and a real-time web dashboard.

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql)
![Redis](https://img.shields.io/badge/Redis-7-DC382D?logo=redis)
![License](https://img.shields.io/badge/license-MIT-green)
![Tests](https://img.shields.io/badge/tests-29%20passing-brightgreen)

---

## Problem & Requirements

### Functional Requirements

- Schedule jobs using standard 5-field cron expressions (`*/15 * * * *`, `0 2 * * *`)
- Execute two job types: shell commands and HTTP webhook callbacks
- Distributed: multiple scheduler and worker processes may run concurrently — no job executes twice per tick
- Retry failed jobs with exponential backoff; move exhausted jobs to a dead-letter queue
- Manual trigger, pause, and resume of any job via REST API
- Full execution history queryable per job

### Non-Functional Requirements

- **Exactly-once scheduling intent** — no duplicate dispatch across N scheduler replicas
- **Sub-second dispatch latency** — job enters worker queue within the scheduler tick; workers consume via blocking pop (no polling sleep)
- **Crash resilience** — scheduler crash must not lose pending jobs; recovery is self-healing on next tick
- **Horizontal worker scaling** — add worker processes without changing scheduler or database configuration

### Capacity Estimates

| Dimension | Estimate |
|-----------|----------|
| Jobs | 1,000 active jobs |
| Average execution frequency | 1 per hour → ~0.28 exec/sec steady state |
| Peak (batch window) | ~280 exec/sec if all jobs align |
| Execution record size | ~500 bytes × 1M executions/year ≈ **500 MB/year** in PostgreSQL |
| Redis queue depth | Typically < 100 items; jobs drain faster than they arrive |
| Scheduler CPU | One SELECT + N lock attempts per 10s tick — negligible |
| Lock TTL overhead | 30s × 1,000 concurrent locks ≈ trivial Redis memory |

---

## Architecture

```
┌────────────────────────────────────────────────┐
│              REST API (port 8080)              │
│  Jobs CRUD · trigger · pause · resume ·        │
│  executions · workers · stats · dashboard      │
└──────────────────┬─────────────────────────────┘
                   │
       ┌───────────▼────────────────┐
       │         Scheduler          │
       │  ┌──────────────────────┐  │
       │  │  robfig/cron parser  │  │
       │  │  next-run calculator │  │
       │  └──────────────────────┘  │
       │  tick every N seconds      │
       │  SELECT jobs WHERE         │
       │    next_run_at <= NOW()    │
       │    AND status = 'active'   │
       │  acquire Redis lock        │
       │  LPUSH to queue            │
       │  UPDATE next_run_at        │
       └───────────┬────────────────┘
                   │
       ┌───────────▼────────────────┐
       │           Redis            │
       │  scheduler:queue:jobs      │  ← LPUSH / BRPOP
       │  scheduler:lock:job:{id}   │  ← SET NX EX 30
       │  scheduler:queue:dead      │  ← dead-letter list
       │  scheduler:worker:{id}     │  ← heartbeat TTL keys
       └───────────┬────────────────┘
                   │ BRPOP (3s timeout)
     ┌─────────────┼──────────────────────┐
     ▼             ▼                      ▼
 Worker 1      Worker 2            Worker N
 goroutines    goroutines          goroutines
 shell · http  shell · http        shell · http
 backoff retry backoff retry       backoff retry
     │             │                      │
     └─────────────┼──────────────────────┘
                   │  INSERT / UPDATE executions
                   ▼
       ┌───────────────────────────┐
       │        PostgreSQL         │
       │  jobs · executions        │
       └───────────────────────────┘
```

**Data flow summary:**

1. Scheduler ticks every `SCHEDULE_INTERVAL_SECONDS` seconds.
2. It queries PostgreSQL for all jobs with `next_run_at <= NOW() AND status = 'active'`.
3. For each due job it attempts `SET scheduler:lock:job:{id} 1 NX EX 30` — only one scheduler node per job wins.
4. The winning node serialises a `QueuedJob` struct and `LPUSH`es it to `scheduler:queue:jobs`, then updates `next_run_at` in PostgreSQL.
5. Workers block on `BRPOP scheduler:queue:jobs 3` — zero latency when a job is present.
6. Workers execute (shell or HTTP), write an execution record to PostgreSQL, and re-enqueue on failure or push to dead-letter on exhaustion.

---

## Key Design Decisions

### 1. Redis SET NX for distributed locking

```
SET scheduler:lock:job:{id}  1  NX  EX 30
```

Only one scheduler node acquires the lock per job per tick, preventing duplicate dispatch even with N running schedulers.

**Trade-off:** The lock TTL is 30 seconds. If a scheduler acquires the lock, enqueues the job, then crashes before releasing, the lock expires and the next tick will re-enqueue — giving **at-least-once** dispatch, not exactly-once. Exactly-once delivery would require a two-phase protocol (mark job as "in-flight" in a transaction, confirm after worker ACK), which was not justified at this scale.

### 2. Separate scheduler and worker processes

The scheduler is single-threaded and CPU-light (one DB query + Redis SET NX per tick). Workers are multi-goroutine and I/O-heavy. Keeping them separate means workers can scale horizontally without adding scheduler lock contention, and a worker crash cannot corrupt the scheduling loop.

**Trade-off:** Two deployment units to manage. Mitigated by a single `docker compose` file that scales workers with `--scale worker=N`.

### 3. PostgreSQL as source of truth, Redis as ephemeral queue

Job definitions and `next_run_at` live in PostgreSQL (durable). The Redis queue is a performance layer — if Redis is wiped, the scheduler rebuilds state from Postgres on its next tick. No job metadata is lost.

**Trade-off:** If the scheduler crashes between `LPUSH` and `UPDATE next_run_at`, the job is in Redis but Postgres still has the old `next_run_at`. On recovery the scheduler re-enqueues it (double-fire risk). Acceptable for non-transactional workloads; addressed in Phase 4 of the roadmap via worker lease keys.

### 4. Blocking pop (BRPOP) on workers

Workers block on `BRPOP scheduler:queue:jobs 3` rather than polling with a sleep. Dispatch latency is effectively the Redis round-trip (~1 ms on loopback) rather than up to `sleep_interval` seconds.

**Trade-off:** Each worker goroutine holds an open Redis connection continuously. Acceptable with connection pooling; Redis supports tens of thousands of concurrent connections.

### 5. Exponential backoff retry

```
delay = 10s × 2^(attempt-1)
```

Attempt 1 → 10s, attempt 2 → 20s, attempt 3 → 40s, … This backs off under transient downstream failures without hammering the target.

**Trade-off:** A job may be delayed by minutes on repeated failure (intentional). The delay is blocking (`time.Sleep`) in the current implementation — a production hardening would push the retry with a future `run_at` timestamp instead, freeing the worker goroutine immediately (tracked in ROADMAP Phase 4).

### 6. Dead-letter queue in Redis

Exhausted jobs (attempt == `max_retries`) are pushed to `scheduler:queue:dead` and their execution record is marked `dead_letter`. The DLQ is inspectable and replayable via a future replay endpoint.

**Trade-off:** The current DLQ is an unbounded Redis list. A production deployment should cap its size or set a TTL on entries (tracked in ROADMAP Phase 4).

### 7. Interface-extracted store layer

`scheduler.go` depends on `jobDB` and `jobQueue` interfaces, not concrete store types. All 29 unit tests use in-memory fakes — zero real DB or Redis required.

**Trade-off:** The interface boundary is at the store level, so integration behaviour (SQL queries, Redis serialisation) is only exercised in CI with real service containers. No in-process integration tests yet (Testcontainers is on the roadmap).

### 8. `next_run_at` persisted in PostgreSQL

Rather than maintaining an in-memory cron wheel, the scheduler persists `next_run_at` per job and queries `WHERE next_run_at <= NOW()` each tick. This survives scheduler restarts with zero state reconstruction.

**Trade-off:** One database round-trip per tick instead of pure in-memory calculation. At 1,000 jobs and a 10-second tick interval this is ~100 queries/second peak — well within PostgreSQL's capacity with a single index on `(status, next_run_at)`.

---

## Distributed Locking Protocol

```
Scheduler tick (runs every SCHEDULE_INTERVAL_SECONDS):

  1. now := time.Now()
  2. jobs := SELECT id, cron_expr, next_run_at, command, ...
             FROM jobs
             WHERE next_run_at <= now AND status = 'active'

  3. For each job in jobs:
       ok := SET scheduler:lock:job:{id}  1  NX  EX 30
       if !ok → skip (another scheduler node won the race)

  4. nextRun := robfig/cron.Next(job.cron_expr, now)
  5. LPUSH scheduler:queue:jobs  {json(QueuedJob)}
  6. UPDATE jobs SET next_run_at = nextRun WHERE id = job.id
  7. (lock expires automatically after 30s)

Worker loop (one goroutine per worker):

  1. job := BRPOP scheduler:queue:jobs  3s  (blocks until available)
  2. INSERT executions (job_id, worker_id, attempt, status='running')
  3. output, err := executeCommand(job.command, job.payload, job.timeout_sec)

  4. if err != nil:
       if job.attempt < job.max_retries:
         sleep(10s × 2^(attempt-1))
         job.attempt++
         LPUSH scheduler:queue:jobs {json(job)}   ← re-enqueue
         UPDATE executions SET status='failed', output=...
       else:
         LPUSH scheduler:queue:dead {json(job)}   ← dead-letter
         UPDATE executions SET status='dead_letter', output=...
     else:
       UPDATE executions SET status='success', output=...
```

---

## Scalability

| Node type | Scaling strategy | Limit |
|-----------|------------------|-------|
| Scheduler | Active-passive via Redis lock; standby nodes do nothing until leader dies | Failover ≤ 30s (lock TTL) |
| Worker | Horizontal — start N worker processes; each races on BRPOP | Redis supports ~1M LPUSH/BRPOP ops/sec; not a bottleneck |
| Queue depth | Redis list; typical depth < 100 during normal operation | Grows only if all workers are saturated |
| PostgreSQL | pgxpool connection pooling; add read replica for stats/history queries | Vertical scale or read replica before sharding |
| Lock contention | O(N×M) SET NX calls per tick (N schedulers × M due jobs); N is typically 2 (active + standby) | Negligible |

---

## Failure Modes

| Failure | Behaviour | Recovery |
|---------|-----------|----------|
| Scheduler crash mid-tick | Redis lock expires after 30s; next scheduler tick re-queries Postgres and re-enqueues missed jobs | Self-healing; at most one missed tick |
| Scheduler crash after LPUSH, before UPDATE next_run_at | Job is in queue but `next_run_at` is stale; next tick re-enqueues the job → double execution possible | Acceptable for shell/idempotent jobs; HTTP webhooks need idempotency keys (Phase 4) |
| Worker crash mid-execution | Execution record left in `running` state; no automatic re-enqueue in current implementation | Planned: scheduler detects stale `running` records and re-enqueues (Phase 4 worker lease) |
| Worker crash before execution | Job is consumed from Redis and lost (BRPOP is destructive) | Planned: at-most-once lease protocol (Phase 4) |
| Redis unavailable | Scheduler cannot acquire locks or enqueue; workers stall; heartbeats stop | Jobs accumulate Postgres backlog; burst re-enqueue on Redis recovery |
| PostgreSQL unavailable | Scheduler cannot query due jobs or update next_run_at; API returns 503 | In-flight jobs may complete; no new jobs dispatched until Postgres recovers |
| Job timeout | `context.WithDeadline` cancels shell `exec.CommandContext` or HTTP request | Execution recorded as `failed`; retry fires if attempts remain |
| DLQ overflow | Redis list grows unbounded if nothing consumes DLQ | Planned: DLQ size cap and replay endpoint (Phase 4) |

---

## Job Types

**Shell command** — any executable reachable by the worker process:

```json
{
  "name": "nightly-backup",
  "cron_expr": "0 2 * * *",
  "command": "/opt/scripts/backup.sh",
  "max_retries": 3,
  "retry_delay_seconds": 60,
  "timeout_seconds": 300
}
```

**HTTP webhook** — any URL starting with `http://` or `https://` is dispatched as an HTTP POST with `Content-Type: application/json`:

```json
{
  "name": "sync-inventory",
  "cron_expr": "*/15 * * * *",
  "command": "https://internal.api/hooks/sync",
  "payload": {"source": "scheduler"},
  "max_retries": 5,
  "retry_delay_seconds": 30,
  "timeout_seconds": 10
}
```

The dispatcher in `internal/worker/executor.go` inspects the command prefix at runtime — no separate `type` field required on the job definition.

---

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/jobs` | Create a job |
| `GET` | `/api/v1/jobs` | List all jobs (paginated) |
| `GET` | `/api/v1/jobs/:id` | Get job by ID |
| `PUT` | `/api/v1/jobs/:id` | Update job fields |
| `DELETE` | `/api/v1/jobs/:id` | Delete a job |
| `POST` | `/api/v1/jobs/:id/trigger` | Execute immediately (bypasses cron) |
| `POST` | `/api/v1/jobs/:id/pause` | Pause scheduling (sets status = paused) |
| `POST` | `/api/v1/jobs/:id/resume` | Resume scheduling (sets status = active) |
| `GET` | `/api/v1/jobs/:id/executions` | Execution history for a job |
| `GET` | `/api/v1/workers` | Active workers (sourced from Redis heartbeat TTL keys) |
| `GET` | `/api/v1/stats` | Queue depth, job counts, execution counts |

### Create job — request body

```json
POST /api/v1/jobs
{
  "name":                 "nightly-backup",
  "cron_expr":            "0 2 * * *",
  "command":              "/opt/scripts/backup.sh",
  "payload":              {},
  "max_retries":          3,
  "retry_delay_seconds":  60,
  "timeout_seconds":      300
}
```

`cron_expr` is validated at creation time using `robfig/cron` — invalid expressions return `400 Bad Request`.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_URL` | `postgres://scheduler:password@localhost:5432/scheduler_db` | PostgreSQL DSN (pgx format) |
| `REDIS_URL` | `redis://localhost:6379` | Redis URL |
| `HTTP_PORT` | `8080` | REST API and dashboard port |
| `WORKER_CONCURRENCY` | `5` | Goroutines per worker process |
| `WORKER_ID` | `worker-1` | Unique identifier for this worker process; used in heartbeat keys |
| `SCHEDULE_INTERVAL_SECONDS` | `10` | Scheduler tick interval |

All variables are read via `spf13/viper` and can also be set in a `.env` file (see `.env.example`).

---

## Running Locally

```bash
# Start dependencies
docker compose -f deployments/docker-compose.yml up -d postgres redis

# Copy config
cp .env.example .env

# Run scheduler (terminal 1)
go run ./cmd/scheduler

# Run worker (terminal 2 — repeat for more workers)
go run ./cmd/worker
```

Dashboard: http://localhost:8080

Scale workers with Docker Compose:

```bash
docker compose -f deployments/docker-compose.yml up -d --scale worker=5
```

---

## Testing

```bash
go test ./... -race -count=1
```

**29 unit tests** across four packages:

| Package | What is covered |
|---------|-----------------|
| `internal/scheduler` | Cron next-run calculation (standard fields, edge cases, invalid expressions); scheduler deduplication logic using fake `jobDB` and `jobQueue`; lock-not-acquired skip path |
| `internal/api` | Handler CRUD (create, list, get, delete, update); trigger / pause / resume transitions; 404 and 400 error paths |
| `internal/worker` | Executor shell success and failure; HTTP webhook success and 4xx failure; retry counter increment; DLQ promotion on exhaustion |
| `internal/models` | Job struct validation |

All tests use in-memory fake implementations of the `JobStore` and `QueueStore` interfaces — no real PostgreSQL or Redis required. CI runs the full suite against real service containers (see `.github/workflows/`).

---

## Tech Stack

| Component | Choice | Why |
|-----------|--------|-----|
| Language | Go 1.22 | Goroutines for worker concurrency; `context` for timeout/cancellation; fast compile |
| Queue / locking | Redis 7 (go-redis/v9) | LPUSH/BRPOP for zero-latency dispatch; SET NX for distributed locks; TTL keys for worker heartbeats |
| Persistence | PostgreSQL 16 (pgx/v5, pgxpool) | ACID job definitions; execution audit log; single index on `(status, next_run_at)` drives all scheduler queries |
| HTTP framework | Gin | Low-overhead routing; binding/validation middleware |
| Cron parsing | robfig/cron v3 | Battle-tested 5-field parser; `schedule.Next()` used for next-run calculation |
| Logging | go.uber.org/zap | Structured JSON logs; fields per job_id / exec_id for log correlation |
| Config | spf13/viper | `.env` file + environment variable overlay |

---

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the full phased plan. Highlights:

- **Phase 2** — Prometheus `/metrics` endpoint, Grafana dashboard, `/healthz` + `/readyz` probes
- **Phase 3** — Timezone support (IANA strings), job DAG dependencies with cycle detection, priority queue (Redis ZADD/BZPOPMIN)
- **Phase 4** — Worker lease keys for crash recovery, DLQ replay endpoint, at-most-once HTTP idempotency keys
- **Phase 5** — Multi-region worker pools with per-region queue keys, PostgreSQL HA (Azure Flexible Server + PgBouncer)
- **Phase 6** — Helm chart + KEDA autoscaler for AKS, event-sourced `job_events` table, Testcontainers integration suite
