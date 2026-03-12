# Distributed Job Scheduler

A production-grade distributed job scheduling system built in Go. Supports cron-based scheduling, distributed worker pools, exponential-backoff retry, dead-letter queues, and a real-time web dashboard.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  REST API / Dashboard            │
│           GET /jobs  POST /jobs  GET /stats      │
└───────────────────────┬─────────────────────────┘
                        │
            ┌───────────▼──────────┐
            │      Scheduler       │  ticks every N seconds
            │  ┌────────────────┐  │  acquires distributed lock
            │  │  robfig/cron   │  │  enqueues due jobs
            │  └────────────────┘  │
            └───────────┬──────────┘
                        │
            ┌───────────▼──────────┐
            │    Redis Queue       │  LPUSH / BRPOP
            │  ┌────────────────┐  │  distributed locks (SET NX)
            │  │  Dead-letter   │  │  worker heartbeats (TTL keys)
            │  └────────────────┘  │
            └───────────┬──────────┘
                        │
     ┌──────────────────▼──────────────────────┐
     │           Worker Pool (N workers)        │
     │   ┌────────┐  ┌────────┐  ┌────────┐   │
     │   │Worker 1│  │Worker 2│  │Worker N│   │
     │   └────────┘  └────────┘  └────────┘   │
     │   Shell cmds · HTTP callbacks · Timeout │
     └──────────────────┬──────────────────────┘
                        │
            ┌───────────▼──────────┐
            │      PostgreSQL      │  jobs · executions · stats
            └──────────────────────┘
```

## Features

- **Cron scheduling** — standard 5-field cron expressions (`0 */6 * * *`)
- **Distributed locking** — Redis SET NX prevents duplicate execution across nodes
- **Worker pool** — configurable concurrency, horizontal scaling via multiple worker processes
- **Job types** — shell commands and HTTP webhook callbacks
- **Retry with backoff** — configurable max retries and delay per job
- **Dead-letter queue** — exhausted jobs moved to Redis dead-letter list
- **Execution history** — full audit trail in PostgreSQL
- **Pause / Resume / Trigger** — manual control via REST API
- **Web dashboard** — real-time stats, job table, worker health

## Quick Start

```bash
# Start dependencies
docker compose -f deployments/docker-compose.yml up -d postgres redis

# Copy and configure environment
cp .env.example .env

# Run scheduler (in one terminal)
go run ./cmd/scheduler

# Run worker (in another terminal — scale as needed)
go run ./cmd/worker
```

Dashboard available at http://localhost:8080

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/jobs` | Create a job |
| `GET` | `/api/v1/jobs` | List jobs (paginated) |
| `GET` | `/api/v1/jobs/:id` | Get job details |
| `DELETE` | `/api/v1/jobs/:id` | Delete a job |
| `POST` | `/api/v1/jobs/:id/trigger` | Manual trigger |
| `POST` | `/api/v1/jobs/:id/pause` | Pause scheduling |
| `POST` | `/api/v1/jobs/:id/resume` | Resume scheduling |
| `GET` | `/api/v1/jobs/:id/executions` | Execution history |
| `GET` | `/api/v1/workers` | Active workers |
| `GET` | `/api/v1/stats` | Queue and job stats |

### Create Job

```json
POST /api/v1/jobs
{
  "name": "nightly-backup",
  "cron_expr": "0 2 * * *",
  "command": "/opt/scripts/backup.sh",
  "max_retries": 3,
  "retry_delay_seconds": 60,
  "timeout_seconds": 300
}
```

HTTP webhook:
```json
{
  "name": "sync-inventory",
  "cron_expr": "*/15 * * * *",
  "command": "https://internal.api/hooks/sync",
  "payload": {"source": "scheduler"}
}
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_URL` | `postgres://scheduler:password@localhost:5432/scheduler_db` | PostgreSQL DSN |
| `REDIS_URL` | `redis://localhost:6379` | Redis URL |
| `HTTP_PORT` | `8080` | API server port |
| `WORKER_CONCURRENCY` | `5` | Jobs per worker process |
| `WORKER_ID` | `worker-1` | Unique worker identifier |
| `SCHEDULE_INTERVAL_SECONDS` | `10` | Scheduler tick interval |

## Running Tests

```bash
go test ./... -race -count=1
```

## Docker Deployment

```bash
# Full stack
docker compose -f deployments/docker-compose.yml up -d

# Scale workers
docker compose -f deployments/docker-compose.yml up -d --scale worker=5
```

## Tech Stack

- **Go 1.22** — goroutines, context cancellation, graceful shutdown
- **PostgreSQL 16** — job definitions, execution audit log
- **Redis 7** — job queue (BRPOP), distributed locks (SET NX), worker heartbeats
- **Gin** — HTTP API framework
- **robfig/cron v3** — cron expression parsing
- **go.uber.org/zap** — structured logging
