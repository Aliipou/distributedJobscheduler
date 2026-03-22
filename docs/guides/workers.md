# Worker Configuration

## Worker Types

### Container Worker (Default)
Runs jobs as Docker containers.

```json
{
  "worker": "container",
  "image": "my-company/etl:latest",
  "command": ["python", "run_etl.py"],
  "resources": {
    "cpu": "1000m",
    "memory": "512Mi"
  }
}
```

### Shell Worker
Runs shell commands on the worker host.

```json
{
  "worker": "shell",
  "command": "cd /opt/etl && python run.py --date {{date}}"
}
```

### HTTP Worker
Makes an HTTP request to trigger a job.

```json
{
  "worker": "http",
  "url": "https://api.example.com/trigger-job",
  "method": "POST",
  "headers": {"Authorization": "Bearer {{secrets.API_KEY}}"},
  "body": {"job": "daily-etl", "date": "{{date}}"}
}
```

## Secrets Injection

```json
{
  "secrets": {
    "DATABASE_URL": {"from": "env", "key": "DATABASE_URL"},
    "API_KEY": {"from": "vault", "path": "secret/etl/api-key"}
  }
}
```

Secrets are injected as environment variables — never logged.

## Worker Scaling

```bash
# Add workers to handle more concurrent jobs
docker compose scale worker=4
```
