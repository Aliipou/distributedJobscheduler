# Cron Expression Guide

distributedJobscheduler uses standard 5-field cron expressions.

## Format

```
┌───── minute (0-59)
│ ┌─── hour (0-23)
│ │ ┌─ day of month (1-31)
│ │ │ ┌ month (1-12)
│ │ │ │ ┌ day of week (0-6, Sun=0)
│ │ │ │ │
* * * * *
```

## Common Patterns

| Expression | Meaning |
|---|---|
| `0 6 * * *` | Every day at 06:00 |
| `*/15 * * * *` | Every 15 minutes |
| `0 9-17 * * 1-5` | Every hour 09:00-17:00, weekdays |
| `0 0 1 * *` | First day of every month at midnight |
| `0 */6 * * *` | Every 6 hours |
| `30 2 * * 0` | Sundays at 02:30 |

## Timezone Support

```json
{
  "schedule": "0 6 * * *",
  "timezone": "Europe/Helsinki"
}
```

All times are interpreted in the specified timezone. Default: UTC.

## Human-Readable Schedules

```json
{"schedule": "@daily"}      // every day at midnight
{"schedule": "@hourly"}     // every hour
{"schedule": "@weekly"}     // sundays at midnight
{"schedule": "@monthly"}    // 1st of month at midnight
{"schedule": "@every 30m"}  // every 30 minutes
```

## Validation

```bash
POST /api/v1/jobs/validate-schedule
{"schedule": "0 6 * * *"}

{"valid": true, "next_5_runs": ["2024-01-16T06:00:00Z", ...]}
```
