# Monitoring Guide

## Built-in Metrics

distributedJobscheduler exposes Prometheus metrics at `/metrics`:

```
# Job execution counts
job_executions_total{job="daily-report",status="success"} 142
job_executions_total{job="daily-report",status="failure"} 3

# Execution duration
job_duration_seconds{job="daily-report",quantile="0.5"} 45.2
job_duration_seconds{job="daily-report",quantile="0.99"} 187.3

# Queue depth
scheduler_queue_depth 5
```

## Grafana Dashboard

Import the bundled dashboard:

```bash
# Import via Grafana API
curl -X POST http://grafana:3000/api/dashboards/import \
  -H "Content-Type: application/json" \
  -d @dashboards/distributed-job-scheduler.json
```

## Alerting Rules

```yaml
# prometheus/rules/scheduler.yml
groups:
  - name: scheduler
    rules:
      - alert: JobFailureRateHigh
        expr: rate(job_executions_total{status="failure"}[10m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High job failure rate for {{ $labels.job }}"

      - alert: JobNotRunning
        expr: time() - job_last_success_timestamp > 3600
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "{{ $labels.job }} hasn't succeeded in 1+ hours"
```
