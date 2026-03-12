package models_test

import (
	"encoding/json"
	"testing"

	"github.com/aliipou/distributed-job-scheduler/internal/models"
)

func TestJobSerialization(t *testing.T) {
	job := &models.Job{
		ID:       "test-id",
		Name:     "Test Job",
		CronExpr: "0 */6 * * *",
		Command:  "echo hello",
		Status:   models.JobStatusActive,
		Payload:  json.RawMessage(`{"key":"value"}`),
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded models.Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != job.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, job.ID)
	}
	if decoded.Status != models.JobStatusActive {
		t.Errorf("Status: got %q, want %q", decoded.Status, models.JobStatusActive)
	}
}

func TestJobStatuses(t *testing.T) {
	statuses := []models.JobStatus{
		models.JobStatusActive,
		models.JobStatusPaused,
		models.JobStatusDeleted,
	}
	for _, s := range statuses {
		if s == "" {
			t.Errorf("empty job status")
		}
	}
}

func TestExecStatuses(t *testing.T) {
	statuses := []models.ExecStatus{
		models.ExecStatusPending,
		models.ExecStatusRunning,
		models.ExecStatusSuccess,
		models.ExecStatusFailed,
		models.ExecStatusDeadLetter,
	}
	for _, s := range statuses {
		if s == "" {
			t.Errorf("empty exec status")
		}
	}
}

func TestQueuedJobSerialization(t *testing.T) {
	q := &models.QueuedJob{
		JobID:      "job-1",
		ExecID:     "exec-1",
		Command:    "echo test",
		Payload:    `{"x":1}`,
		Attempt:    1,
		MaxRetries: 3,
		TimeoutSec: 300,
	}
	data, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got models.QueuedJob
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.JobID != q.JobID || got.MaxRetries != q.MaxRetries {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, q)
	}
}
