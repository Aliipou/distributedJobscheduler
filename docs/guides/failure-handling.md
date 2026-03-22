# Failure Handling

## Automatic Retries

```json
{
  "name": "etl-job",
  "schedule": "0 2 * * *",
  "retries": 3,
  "retry_delay_seconds": 300,
  "retry_backoff": "exponential"
}
```

Retry delays (exponential): 5 min, 10 min, 20 min.

## Dead Letter Queue

Failed jobs after all retries go to the dead letter queue:

```bash
GET /api/v1/dlq?limit=50

POST /api/v1/dlq/{execution_id}/retry  # re-run a failed job
DELETE /api/v1/dlq/{execution_id}       # dismiss
```

## Alerting on Failure

```json
{
  "name": "etl-job",
  "on_failure": {
    "channel": "slack",
    "webhook": "https://hooks.slack.com/...",
    "message": "{{job}} failed after {{retries}} retries. Exit code: {{exit_code}}"
  }
}
```

## Timeout Handling

```json
{
  "timeout_seconds": 3600,
  "timeout_action": "kill"  // or "graceful" (SIGTERM then SIGKILL after 30s)
}
```

## Concurrency Control

```json
{
  "concurrency": 1,  // max 1 concurrent execution
  "on_concurrent": "skip"  // or "queue" or "kill_previous"
}
```
