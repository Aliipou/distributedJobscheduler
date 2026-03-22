# Quickstart

```bash
docker compose up --build -d
curl -X POST http://localhost:8080/api/v1/jobs -H 'Content-Type: application/json' -d '{"name": "my-job", "schedule": "0 6 * * *", "command": ["python", "run.py"]}'
```

Jobs run at 06:00 UTC daily. View history at `GET /api/v1/jobs/my-job/executions`.
