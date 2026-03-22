# API Guide

## Create a Job

```bash
POST /api/v1/jobs
Content-Type: application/json

{
  "name": "daily-report",
  "schedule": "0 6 * * *",
  "image": "my-company/report-generator:latest",
  "command": ["python", "generate_report.py"],
  "timeout_seconds": 300,
  "retries": 2,
  "resources": {
    "cpu": "500m",
    "memory": "256Mi"
  }
}
```

## List Jobs

```bash
GET /api/v1/jobs?status=active&limit=50
```

## Get Job Executions

```bash
GET /api/v1/jobs/daily-report/executions?limit=10
```

Response:

```json
{
  "executions": [
    {
      "id": "exec-001",
      "job": "daily-report",
      "status": "completed",
      "started_at": "2024-01-15T06:00:00Z",
      "completed_at": "2024-01-15T06:02:34Z",
      "exit_code": 0
    }
  ]
}
```

## Trigger Manually

```bash
POST /api/v1/jobs/daily-report/trigger
```

## Cancel Execution

```bash
DELETE /api/v1/executions/exec-001
```
