# Distributed Job Scheduler — Roadmap

A practical checklist for hardening and extending this scheduler from a
production-ready single-region system into a multi-region, self-healing platform.

---

## Phase 1 — Foundation (complete ✓)

- [x] Cron-based scheduling with robfig/cron v3 (5-field expressions)
- [x] Redis distributed locking (SET NX) — prevents duplicate execution across nodes
- [x] Worker pool with configurable concurrency per worker process
- [x] Job types: shell command + HTTP webhook callbacks
- [x] Exponential-backoff retry with per-job max retries and delay
- [x] Dead-letter queue in Redis for exhausted jobs
- [x] Full execution audit log in PostgreSQL
- [x] Pause / Resume / Manual trigger via REST API
- [x] Interface-extracted store layer — fully unit-testable without real DB/Redis
- [x] 29 unit tests (handlers, scheduler) with fake in-memory implementations
- [x] GitHub Actions CI with Postgres + Redis service containers

---

## Phase 2 — Observability & Reliability

### Execution Metrics
- [ ] Expose `/metrics` in Prometheus text format
  - `scheduler_jobs_total{status}` counter
  - `scheduler_execution_duration_seconds` histogram
  - `scheduler_queue_depth` gauge
  - `scheduler_worker_active` gauge
- [ ] Grafana dashboard JSON in `deployments/grafana/`

### Structured Audit Logging
- [ ] Log every scheduler decision (acquired lock / skipped / enqueued) as structured events
- [ ] Correlation ID per job run propagated through log fields and DB execution record
- [ ] Log level configurable per job (verbose debug for development jobs)

### Health Endpoints
- [ ] `GET /healthz` — liveness (always 200)
- [ ] `GET /readyz` — readiness (checks Postgres ping + Redis ping)
- [ ] Worker heartbeat visible in `GET /api/v1/workers` with last-seen age

---

## Phase 3 — Advanced Scheduling

### Timezone Support
- [ ] `timezone` field on job definition (IANA tz string, e.g. `Europe/Helsinki`)
- [ ] Validate timezone on job creation; default to `UTC`
- [ ] Display next-run time in job's local timezone on dashboard

### Dependent Jobs (DAG)
- [ ] `depends_on: [job_id, ...]` field on job definition
- [ ] Scheduler checks all upstream jobs completed successfully within window
- [ ] Cycle detection at job creation time (DFS on dependency graph)
- [ ] DAG visualisation on dashboard (SVG or Mermaid)

### Job Priorities
- [ ] Priority field (1–10) on job definition
- [ ] Redis sorted set keyed by priority + enqueue time (ZADD)
- [ ] Worker pops highest-priority job available (BZPOPMIN)

### Rate Limiting
- [ ] Max concurrent executions per job (not per worker)
- [ ] Global worker-pool concurrency cap enforced via Redis semaphore (SETNX counter)
- [ ] Configurable cooldown between executions of the same job

---

## Phase 4 — Fault Tolerance

### Job Timeout & Cancellation
- [ ] Context with deadline passed to shell/HTTP executor; kill process on timeout
- [ ] Timeout stored in execution record; surfaced on dashboard
- [ ] Graceful SIGTERM → hard SIGKILL after 5s for shell jobs

### Distributed Crash Recovery
- [ ] Worker lease keys in Redis (TTL = 2 × heartbeat interval)
- [ ] Scheduler re-enqueues jobs whose worker lease expired mid-execution
- [ ] At-most-once guarantee for HTTP webhooks (idempotency key in `X-Idempotency-Key` header)

### DLQ Replay
- [ ] `POST /api/v1/jobs/:id/replay-dlq` moves job from dead-letter back to queue
- [ ] DLQ browser in dashboard showing last N exhausted executions
- [ ] Optional auto-replay on manual trigger with reset retry counter

---

## Phase 5 — Multi-Region

### Active-Active Scheduler Nodes
- [ ] Each scheduler node holds a distributed leader lease (Redis SETNX with TTL)
- [ ] Non-leader nodes run in standby; only leader enqueues due jobs
- [ ] Leader election event logged and exposed in `/api/v1/stats`

### Cross-Region Worker Pools
- [ ] Worker `region` tag in registration; job `target_region` constraint
- [ ] Separate Redis queue keys per region (`queue:jobs:eu-north`, `queue:jobs:us-east`)
- [ ] Routing layer in scheduler selects queue based on job constraint

### PostgreSQL HA
- [ ] Move to Azure Database for PostgreSQL Flexible Server with zone-redundant HA
- [ ] Connection pooling via PgBouncer sidecar (transaction mode)
- [ ] Read replica for dashboard queries (execution history, stats)

---

## Phase 6 — Cloud & Operations

### Kubernetes Deployment (AKS)
- [ ] Helm chart: Scheduler Deployment (1 replica + leader election), Worker Deployment (N replicas)
- [ ] KEDA ScaledObject for workers based on Redis queue depth
- [ ] PodDisruptionBudget for worker tier (min 1 available)
- [ ] ConfigMap for environment variables; Secret for DB/Redis DSNs from Key Vault CSI driver

### Event Sourcing
- [ ] Append-only `job_events` table (created / updated / triggered / paused / deleted)
- [ ] Rebuild current job state by replaying events (event-sourced read model)
- [ ] Snapshots every 100 events per job for fast bootstrap

### CI/CD Improvements
- [ ] Integration test job with Testcontainers (Postgres + Redis spun up in-process)
- [ ] Trivy scan on Docker image in CI
- [ ] Automated release tagging + CHANGELOG generation on `main` merge
- [ ] Helm chart publish to GitHub Pages OCI registry
