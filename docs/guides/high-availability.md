# High Availability Guide

## Architecture

distributedJobscheduler uses leader election for HA:

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Scheduler 1  │     │  Scheduler 2  │     │  Scheduler 3  │
│   (LEADER)   │     │   (standby)  │     │   (standby)  │
└──────┬───────┘     └──────────────┘     └──────────────┘
       │
       │ (leader election via PostgreSQL advisory lock)
       ▼
┌──────────────┐
│  PostgreSQL  │ (shared state: jobs, executions, locks)
└──────────────┘
```

Only the leader schedules and dispatches jobs. Standbys take over within 30 seconds of leader failure.

## Deployment

```yaml
# docker-compose.yml
services:
  scheduler:
    image: aliipou/distributed-job-scheduler:latest
    replicas: 3
    environment:
      - POSTGRES_DSN=postgresql://scheduler:pass@db/scheduler
      - LEADER_ELECTION_TIMEOUT=30
```

## Split-Brain Prevention

The leader holds a PostgreSQL advisory lock with a heartbeat every 10 seconds. If the lock lapses (network partition, crash), a standby acquires it within `LEADER_ELECTION_TIMEOUT` seconds.

No two schedulers can simultaneously hold the lock — PostgreSQL guarantees this.

## Worker HA

Workers are stateless and can run on any host:

```yaml
worker:
  replicas: 4
```

If a worker crashes mid-job, the job is marked failed and retried according to its retry policy.
