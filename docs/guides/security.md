# Security Guide

## Authentication

All API endpoints require authentication:

```bash
# API Key (recommended for service-to-service)
curl -H "X-API-Key: your-key" http://scheduler:8080/api/v1/jobs

# JWT (for user-facing UIs)
curl -H "Authorization: Bearer eyJ..." http://scheduler:8080/api/v1/jobs
```

## API Key Management

```bash
POST /api/v1/admin/api-keys
{"name": "ci-pipeline", "permissions": ["jobs.read", "jobs.trigger"]}
```

Permissions:
- `jobs.read` — List and view jobs/executions
- `jobs.write` — Create/modify/delete jobs
- `jobs.trigger` — Manually trigger jobs
- `admin` — Full access including user management

## Secrets in Jobs

Never put secrets in job commands. Use environment injection:

```json
{
  "command": ["python", "sync.py"],
  "secrets": {
    "DATABASE_URL": {"from": "env", "key": "SCHEDULER_DB_URL"}
  }
}
```

Secrets appear as environment variables in the container. They are never logged.

## Network Isolation

```yaml
# docker-compose.yml — isolate scheduler from public internet
services:
  scheduler:
    networks:
      - internal
  # only expose API through a proxy
  nginx:
    networks:
      - internal
      - public
```

## Audit Log

All state changes are logged:
```
2024-01-15 10:23:45 user=admin action=job.create job=daily-etl
2024-01-15 10:24:00 user=ci-pipeline action=job.trigger job=daily-etl
```
