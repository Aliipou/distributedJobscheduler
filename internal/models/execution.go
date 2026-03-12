package models

import "time"

type Execution struct {
	ID         string     `json:"id" db:"id"`
	JobID      string     `json:"job_id" db:"job_id"`
	WorkerID   string     `json:"worker_id" db:"worker_id"`
	Status     ExecStatus `json:"status" db:"status"`
	Attempt    int        `json:"attempt" db:"attempt"`
	Output     string     `json:"output" db:"output"`
	Error      string     `json:"error" db:"error"`
	StartedAt  time.Time  `json:"started_at" db:"started_at"`
	FinishedAt *time.Time `json:"finished_at" db:"finished_at"`
}

type QueuedJob struct {
	JobID      string `json:"job_id"`
	ExecID     string `json:"exec_id"`
	Command    string `json:"command"`
	Payload    string `json:"payload"`
	Attempt    int    `json:"attempt"`
	MaxRetries int    `json:"max_retries"`
	TimeoutSec int    `json:"timeout_seconds"`
}
