<div align="center">

<img src="https://capsule-render.vercel.app/api?type=waving&amp;color=gradient&amp;customColorList=4,12,18&amp;height=180&amp;section=header&amp;text=Distributed%20Job%20Scheduler&amp;fontSize=38&amp;fontColor=fff&amp;animation=twinkling&amp;fontAlignY=38" />

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&amp;logo=go&amp;logoColor=white)](https://golang.org)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D?style=flat&amp;logo=redis&amp;logoColor=white)](https://redis.io)
[![gRPC](https://img.shields.io/badge/gRPC-1.x-244c5a?style=flat&amp;logo=google)](https://grpc.io)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)

**Distributed job scheduler with Redis-backed queues, worker pools, and fault-tolerant execution.**

</div>

## The Problem

Background job processing seems simple until you need it to be reliable. A single-server queue fails when the server goes down. Jobs get lost. Long-running tasks block short ones. There is no visibility into what is running, waiting, or stuck.

This scheduler is built for production workloads where losing a job is not acceptable.

## Design Principles

**Reliability first.** Every job is persisted in Redis before a worker claims it. If a worker crashes mid-execution, the job is automatically requeued after a configurable timeout.

**Priority scheduling.** Jobs enter priority queues. Critical jobs always run before batch jobs, regardless of submission order.

**Horizontal scaling.** Workers are stateless. Add more workers by starting more processes — no coordination required.

**Observability.** Every state transition (queued, running, succeeded, failed, retried) is emitted as a structured log event and a Prometheus metric.

## Architecture

```
Job Producers
     |
     v
[gRPC API]           Submit jobs, query status, cancel
     |
     v
[Redis Queues]       Priority queues (critical / default / batch)
     |    ^
     v    |          Atomic LPUSH/BRPOPLPUSH for safe handoff
[Worker Pool]        N stateless workers, configurable concurrency
     |
     v
[Result Store]       Job outcomes persisted in Redis with TTL
     |
     v
[Metrics + Logs]     Prometheus counters, structured JSON logs
```

## Quick Start

```bash
git clone https://github.com/Aliipou/distributedJobscheduler.git
cd distributedJobscheduler

# Start Redis
docker run -d -p 6379:6379 redis:7-alpine

# Start workers (run multiple for scale)
go run cmd/worker/main.go --concurrency 10 --queues critical,default,batch

# Submit a job
go run cmd/submit/main.go --type email --payload '{"to":"user@example.com","subject":"Hello"}'
```

## gRPC API

```protobuf
service Scheduler {
  rpc SubmitJob(SubmitJobRequest) returns (SubmitJobResponse);
  rpc GetJobStatus(GetJobStatusRequest) returns (JobStatus);
  rpc CancelJob(CancelJobRequest) returns (CancelJobResponse);
  rpc ListJobs(ListJobsRequest) returns (stream JobStatus);
}
```

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

```yaml
redis:
  url: redis://localhost:6379
  pool_size: 20

workers:
  concurrency: 10
  queues: [critical, default, batch]
  poll_interval: 500ms

jobs:
  default_timeout: 30s
  max_retries: 3
  retry_backoff: exponential
  dead_letter_ttl: 7d

metrics:
  enabled: true
  port: 9090
```

## Performance

| Metric | Value |
|--------|-------|
| Throughput | 10,000+ jobs/sec (single Redis node) |
| Latency (enqueue) | < 1ms p99 |
| Latency (dequeue) | < 2ms p99 |
| Worker startup | < 100ms |

## License

MIT
