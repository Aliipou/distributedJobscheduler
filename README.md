<div align="center">

<img src="https://capsule-render.vercel.app/api?type=waving&amp;color=gradient&amp;customColorList=4,12,18&amp;height=180&amp;section=header&amp;text=Distributed%20Job%20Scheduler&amp;fontSize=38&amp;fontColor=fff&amp;animation=twinkling&amp;fontAlignY=38" />

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&amp;logo=go&amp;logoColor=white)](https://golang.org)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D?style=flat&amp;logo=redis&amp;logoColor=white)](https://redis.io)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-4169E1?style=flat&amp;logo=postgresql&amp;logoColor=white)](https://postgresql.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)

**Distributed job scheduler with Redis-backed queues, Postgres persistence, worker pools, and fault-tolerant execution. REST API via Gin.**

</div>

## The Problem

Background job processing seems simple until you need it to be reliable. A single-server queue fails when the server goes down. Jobs get lost. Long-running tasks block short ones. There is no visibility into what is running, waiting, or stuck.

This scheduler is built for production workloads where losing a job is not acceptable.

## Design Principles

**Reliability first.** Every job is persisted in PostgreSQL and enqueued in Redis. If a worker crashes mid-execution, the job can be requeued after a configurable timeout.

**Cron-based scheduling.** Jobs are defined with cron expressions and automatically scheduled when due.

**Horizontal scaling.** Workers are stateless. Add more workers by starting more processes -- no coordination required.

**Observability.** Every state transition (queued, running, succeeded, failed, retried) is emitted as a structured log event.

## Architecture

```
Job Producers
     |
     v
[REST API (Gin)]     Create jobs, query status, trigger, pause/resume
     |
     v
[PostgreSQL]         Job definitions, execution history, cron state
     |
     v
[Scheduler Loop]     Polls for due jobs, enqueues to Redis
     |
     v
[Redis Queue]        FIFO job queue with distributed locking
     |
     v
[Worker Pool]        N stateless workers, configurable concurrency
     |
     v
[Execution Store]    Results persisted in PostgreSQL
```

## Quick Start

```bash
git clone https://github.com/Aliipou/distributedJobscheduler.git
cd distributedJobscheduler

# Start dependencies
docker compose -f deployments/docker-compose.yml up -d

# Start the scheduler + API server
go run cmd/scheduler/main.go

# Start workers (run multiple for scale)
go run cmd/worker/main.go

# Create a job via the REST API
curl -X POST http://localhost:8080/api/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-job","cron_expr":"*/5 * * * *","command":"echo hello"}'
```

## REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/jobs` | Create a new job |
| GET | `/api/v1/jobs` | List all jobs |
| GET | `/api/v1/jobs/:id` | Get job details |
| DELETE | `/api/v1/jobs/:id` | Delete a job |
| POST | `/api/v1/jobs/:id/trigger` | Manually trigger a job |
| POST | `/api/v1/jobs/:id/pause` | Pause a job |
| POST | `/api/v1/jobs/:id/resume` | Resume a paused job |
| GET | `/api/v1/jobs/:id/executions` | List executions for a job |
| GET | `/api/v1/executions` | List all executions |
| GET | `/api/v1/workers` | List active workers |
| GET | `/api/v1/stats` | Get system stats |

## Job Lifecycle

```
Submitted -> Queued -> Running -> Succeeded
                            |
                            v
                         Failed -> Retrying (up to max_retries)
                                       |
                                       v
                                   Dead (moved to dead-letter queue)
```

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_URL` | `redis://localhost:6379` | Redis connection string |
| `DATABASE_URL` | | PostgreSQL connection string |
| `HTTP_PORT` | `8080` | API server port |
| `SCHEDULE_INTERVAL` | `10s` | How often the scheduler checks for due jobs |

## Performance

Formal benchmarks have not been run yet. See [BENCHMARKS.md](BENCHMARKS.md) for plans.

## License

MIT
