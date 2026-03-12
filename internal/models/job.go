package models

import (
	"encoding/json"
	"time"
)

type JobStatus string
type ExecStatus string

const (
	JobStatusActive  JobStatus = "active"
	JobStatusPaused  JobStatus = "paused"
	JobStatusDeleted JobStatus = "deleted"

	ExecStatusPending    ExecStatus = "pending"
	ExecStatusRunning    ExecStatus = "running"
	ExecStatusSuccess    ExecStatus = "success"
	ExecStatusFailed     ExecStatus = "failed"
	ExecStatusDeadLetter ExecStatus = "dead_letter"
)

type Job struct {
	ID                 string          `json:"id" db:"id"`
	Name               string          `json:"name" db:"name"`
	CronExpr           string          `json:"cron_expr" db:"cron_expr"`
	Command            string          `json:"command" db:"command"`
	Payload            json.RawMessage `json:"payload" db:"payload"`
	MaxRetries         int             `json:"max_retries" db:"max_retries"`
	RetryDelaySeconds  int             `json:"retry_delay_seconds" db:"retry_delay_seconds"`
	TimeoutSeconds     int             `json:"timeout_seconds" db:"timeout_seconds"`
	Status             JobStatus       `json:"status" db:"status"`
	CreatedAt          time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
	NextRunAt          *time.Time      `json:"next_run_at" db:"next_run_at"`
	LastRunAt          *time.Time      `json:"last_run_at" db:"last_run_at"`
}

type CreateJobRequest struct {
	Name              string          `json:"name" binding:"required"`
	CronExpr          string          `json:"cron_expr" binding:"required"`
	Command           string          `json:"command" binding:"required"`
	Payload           json.RawMessage `json:"payload"`
	MaxRetries        int             `json:"max_retries"`
	RetryDelaySeconds int             `json:"retry_delay_seconds"`
	TimeoutSeconds    int             `json:"timeout_seconds"`
}

type UpdateJobRequest struct {
	Name              *string         `json:"name"`
	CronExpr          *string         `json:"cron_expr"`
	Command           *string         `json:"command"`
	Payload           json.RawMessage `json:"payload"`
	MaxRetries        *int            `json:"max_retries"`
	RetryDelaySeconds *int            `json:"retry_delay_seconds"`
	TimeoutSeconds    *int            `json:"timeout_seconds"`
}

type JobStats struct {
	TotalJobs    int64 `json:"total_jobs"`
	ActiveJobs   int64 `json:"active_jobs"`
	PausedJobs   int64 `json:"paused_jobs"`
	TotalExecs   int64 `json:"total_executions"`
	SuccessExecs int64 `json:"successful_executions"`
	FailedExecs  int64 `json:"failed_executions"`
	DeadLetters  int64 `json:"dead_letters"`
}
