package models

import "time"

type WorkerInfo struct {
	ID          string    `json:"id"`
	Hostname    string    `json:"hostname"`
	Status      string    `json:"status"`
	CurrentJob  string    `json:"current_job,omitempty"`
	LastSeen    time.Time `json:"last_seen"`
	JobsHandled int64     `json:"jobs_handled"`
}
