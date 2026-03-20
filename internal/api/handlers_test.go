package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/api"
	"github.com/aliipou/distributed-job-scheduler/internal/models"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── Fakes ─────────────────────────────────────────────────────────────────────

type fakeJobStore struct {
	jobs       map[string]*models.Job
	executions []*models.Execution
	stats      *models.JobStats
	err        error // inject errors
}

func newFakeJobStore() *fakeJobStore {
	return &fakeJobStore{
		jobs:  make(map[string]*models.Job),
		stats: &models.JobStats{},
	}
}

func (f *fakeJobStore) CreateJob(_ context.Context, req *models.CreateJobRequest, nextRun time.Time) (*models.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	job := &models.Job{
		ID:             "job-abc",
		Name:           req.Name,
		CronExpr:       req.CronExpr,
		Command:        req.Command,
		Payload:        req.Payload,
		MaxRetries:     req.MaxRetries,
		TimeoutSeconds: req.TimeoutSeconds,
		Status:         models.JobStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		NextRunAt:      &nextRun,
	}
	if job.Payload == nil {
		job.Payload = []byte("{}")
	}
	f.jobs[job.ID] = job
	return job, nil
}

func (f *fakeJobStore) GetJob(_ context.Context, id string) (*models.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	j, ok := f.jobs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return j, nil
}

func (f *fakeJobStore) ListJobs(_ context.Context, limit, offset int) ([]*models.Job, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	var jobs []*models.Job
	i := 0
	for _, j := range f.jobs {
		if i >= offset && len(jobs) < limit {
			jobs = append(jobs, j)
		}
		i++
	}
	return jobs, int64(len(f.jobs)), nil
}

func (f *fakeJobStore) UpdateJobStatus(_ context.Context, id string, status models.JobStatus) error {
	if f.err != nil {
		return f.err
	}
	if j, ok := f.jobs[id]; ok {
		j.Status = status
	}
	return nil
}

func (f *fakeJobStore) UpdateNextRun(_ context.Context, id string, nextRun time.Time) error {
	if f.err != nil {
		return f.err
	}
	if j, ok := f.jobs[id]; ok {
		j.NextRunAt = &nextRun
	}
	return nil
}

func (f *fakeJobStore) GetDueJobs(_ context.Context, now time.Time) ([]*models.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	var due []*models.Job
	for _, j := range f.jobs {
		if j.Status == models.JobStatusActive && j.NextRunAt != nil && !j.NextRunAt.After(now) {
			due = append(due, j)
		}
	}
	return due, nil
}

func (f *fakeJobStore) CreateExecution(_ context.Context, jobID, workerID string, attempt int) (*models.Execution, error) {
	if f.err != nil {
		return nil, f.err
	}
	e := &models.Execution{
		ID:       "exec-1",
		JobID:    jobID,
		WorkerID: workerID,
		Status:   models.ExecStatusPending,
		Attempt:  attempt,
	}
	f.executions = append(f.executions, e)
	return e, nil
}

func (f *fakeJobStore) UpdateExecution(_ context.Context, id string, status models.ExecStatus, output, errMsg string) error {
	return f.err
}

func (f *fakeJobStore) UpdateExecutionStatus(_ context.Context, id string, status models.ExecStatus) error {
	return f.err
}

func (f *fakeJobStore) ListExecutions(_ context.Context, jobID string, limit int) ([]*models.Execution, error) {
	if f.err != nil {
		return nil, f.err
	}
	var result []*models.Execution
	for _, e := range f.executions {
		if jobID == "" || e.JobID == jobID {
			result = append(result, e)
		}
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (f *fakeJobStore) GetStats(_ context.Context) (*models.JobStats, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.stats, nil
}

type fakeQueueStore struct {
	queue   []*models.QueuedJob
	dead    []*models.QueuedJob
	workers []*models.WorkerInfo
	err     error
}

func (f *fakeQueueStore) AcquireJobLock(_ context.Context, _ string) (bool, error) {
	return true, f.err
}

func (f *fakeQueueStore) ReleaseJobLock(_ context.Context, _ string) error { return f.err }

func (f *fakeQueueStore) EnqueueJob(_ context.Context, job *models.QueuedJob) error {
	if f.err != nil {
		return f.err
	}
	f.queue = append(f.queue, job)
	return nil
}

func (f *fakeQueueStore) DequeueJob(_ context.Context, _ time.Duration) (*models.QueuedJob, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.queue) == 0 {
		return nil, nil
	}
	job := f.queue[len(f.queue)-1]
	f.queue = f.queue[:len(f.queue)-1]
	return job, nil
}

func (f *fakeQueueStore) EnqueueDeadLetter(_ context.Context, job *models.QueuedJob) error {
	if f.err != nil {
		return f.err
	}
	f.dead = append(f.dead, job)
	return nil
}

func (f *fakeQueueStore) RegisterWorker(_ context.Context, info *models.WorkerInfo) error {
	return f.err
}

func (f *fakeQueueStore) GetWorkers(_ context.Context) ([]*models.WorkerInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.workers, nil
}

func (f *fakeQueueStore) QueueLength(_ context.Context) (int64, error) {
	return int64(len(f.queue)), f.err
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newTestRouter(jobs *fakeJobStore, queue *fakeQueueStore) *gin.Engine {
	log := zap.NewNop()
	engine := gin.New()
	engine.Use(gin.Recovery())
	h := api.NewHandler(jobs, queue, log)
	h.RegisterRoutes(engine)
	return engine
}

func doRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── CreateJob tests ───────────────────────────────────────────────────────────

func TestCreateJob_Success(t *testing.T) {
	jobs := newFakeJobStore()
	router := newTestRouter(jobs, &fakeQueueStore{})

	body := map[string]interface{}{
		"name":      "nightly-backup",
		"cron_expr": "0 2 * * *",
		"command":   "backup.sh",
	}
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp models.Job
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "nightly-backup" {
		t.Errorf("name: got %q, want %q", resp.Name, "nightly-backup")
	}
	if resp.Status != models.JobStatusActive {
		t.Errorf("status: got %q, want active", resp.Status)
	}
}

func TestCreateJob_MissingFields(t *testing.T) {
	router := newTestRouter(newFakeJobStore(), &fakeQueueStore{})

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"missing_name", map[string]interface{}{"cron_expr": "* * * * *", "command": "x"}},
		{"missing_cron", map[string]interface{}{"name": "x", "command": "x"}},
		{"missing_command", map[string]interface{}{"name": "x", "cron_expr": "* * * * *"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := doRequest(t, router, http.MethodPost, "/api/v1/jobs", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestCreateJob_InvalidCron(t *testing.T) {
	router := newTestRouter(newFakeJobStore(), &fakeQueueStore{})
	body := map[string]interface{}{
		"name":      "x",
		"cron_expr": "not valid cron",
		"command":   "echo x",
	}
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs", body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateJob_StoreError(t *testing.T) {
	jobs := newFakeJobStore()
	jobs.err = errors.New("db unavailable")
	router := newTestRouter(jobs, &fakeQueueStore{})
	body := map[string]interface{}{
		"name":      "x",
		"cron_expr": "* * * * *",
		"command":   "echo x",
	}
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs", body)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ListJobs tests ────────────────────────────────────────────────────────────

func TestListJobs_Empty(t *testing.T) {
	router := newTestRouter(newFakeJobStore(), &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/jobs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["total"].(float64) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

func TestListJobs_WithJobs(t *testing.T) {
	jobs := newFakeJobStore()
	next := time.Now().Add(time.Hour)
	jobs.jobs["job-1"] = &models.Job{ID: "job-1", Name: "job1", Status: models.JobStatusActive, NextRunAt: &next}
	jobs.jobs["job-2"] = &models.Job{ID: "job-2", Name: "job2", Status: models.JobStatusPaused, NextRunAt: &next}

	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/jobs?limit=10&offset=0", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["total"].(float64) != 2 {
		t.Errorf("expected total=2, got %v", resp["total"])
	}
}

func TestListJobs_StoreError(t *testing.T) {
	jobs := newFakeJobStore()
	jobs.err = errors.New("db down")
	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/jobs", nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── GetJob tests ──────────────────────────────────────────────────────────────

func TestGetJob_Found(t *testing.T) {
	jobs := newFakeJobStore()
	next := time.Now().Add(time.Hour)
	jobs.jobs["job-1"] = &models.Job{ID: "job-1", Name: "backup", Status: models.JobStatusActive, NextRunAt: &next}

	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/jobs/job-1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp models.Job
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != "job-1" {
		t.Errorf("id: got %q, want job-1", resp.ID)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	router := newTestRouter(newFakeJobStore(), &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/jobs/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── DeleteJob tests ───────────────────────────────────────────────────────────

func TestDeleteJob_Success(t *testing.T) {
	jobs := newFakeJobStore()
	next := time.Now().Add(time.Hour)
	jobs.jobs["job-1"] = &models.Job{ID: "job-1", Name: "x", Status: models.JobStatusActive, NextRunAt: &next}

	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodDelete, "/api/v1/jobs/job-1", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if jobs.jobs["job-1"].Status != models.JobStatusDeleted {
		t.Error("expected status to be deleted")
	}
}

func TestDeleteJob_StoreError(t *testing.T) {
	jobs := newFakeJobStore()
	jobs.err = errors.New("db error")
	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodDelete, "/api/v1/jobs/any", nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── TriggerJob tests ──────────────────────────────────────────────────────────

func TestTriggerJob_Success(t *testing.T) {
	jobs := newFakeJobStore()
	queue := &fakeQueueStore{}
	next := time.Now().Add(time.Hour)
	jobs.jobs["job-1"] = &models.Job{
		ID: "job-1", Name: "x", Command: "echo hi",
		Status: models.JobStatusActive, NextRunAt: &next,
		Payload: []byte("{}"),
	}

	router := newTestRouter(jobs, queue)
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs/job-1/trigger", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(queue.queue) != 1 {
		t.Errorf("expected 1 queued job, got %d", len(queue.queue))
	}
}

func TestTriggerJob_NotFound(t *testing.T) {
	router := newTestRouter(newFakeJobStore(), &fakeQueueStore{})
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs/ghost/trigger", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTriggerJob_EnqueueError(t *testing.T) {
	jobs := newFakeJobStore()
	queue := &fakeQueueStore{err: errors.New("redis down")}
	next := time.Now().Add(time.Hour)
	jobs.jobs["job-1"] = &models.Job{
		ID: "job-1", Name: "x", Command: "echo hi",
		Status: models.JobStatusActive, NextRunAt: &next, Payload: []byte("{}"),
	}
	router := newTestRouter(jobs, queue)
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs/job-1/trigger", nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── PauseJob / ResumeJob tests ────────────────────────────────────────────────

func TestPauseJob_Success(t *testing.T) {
	jobs := newFakeJobStore()
	next := time.Now().Add(time.Hour)
	jobs.jobs["job-1"] = &models.Job{ID: "job-1", Name: "x", Status: models.JobStatusActive, NextRunAt: &next}

	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs/job-1/pause", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if jobs.jobs["job-1"].Status != models.JobStatusPaused {
		t.Error("expected paused status")
	}
}

func TestResumeJob_Success(t *testing.T) {
	jobs := newFakeJobStore()
	next := time.Now().Add(time.Hour)
	jobs.jobs["job-1"] = &models.Job{ID: "job-1", Name: "x", Status: models.JobStatusPaused, NextRunAt: &next}

	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs/job-1/resume", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if jobs.jobs["job-1"].Status != models.JobStatusActive {
		t.Error("expected active status")
	}
}

// ── Executions tests ──────────────────────────────────────────────────────────

func TestListJobExecutions_Success(t *testing.T) {
	jobs := newFakeJobStore()
	next := time.Now().Add(time.Hour)
	jobs.jobs["job-1"] = &models.Job{ID: "job-1", Status: models.JobStatusActive, NextRunAt: &next}
	jobs.executions = []*models.Execution{
		{ID: "e1", JobID: "job-1", Status: models.ExecStatusSuccess},
		{ID: "e2", JobID: "job-1", Status: models.ExecStatusFailed},
	}

	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/jobs/job-1/executions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	execs := resp["executions"].([]interface{})
	if len(execs) != 2 {
		t.Errorf("expected 2 executions, got %d", len(execs))
	}
}

func TestListExecutions_All(t *testing.T) {
	jobs := newFakeJobStore()
	jobs.executions = []*models.Execution{
		{ID: "e1", JobID: "job-1", Status: models.ExecStatusSuccess},
		{ID: "e2", JobID: "job-2", Status: models.ExecStatusFailed},
	}

	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/executions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["executions"] == nil {
		t.Error("expected executions key")
	}
}

// ── Workers and Stats tests ───────────────────────────────────────────────────

func TestListWorkers_Empty(t *testing.T) {
	router := newTestRouter(newFakeJobStore(), &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/workers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListWorkers_WithWorkers(t *testing.T) {
	queue := &fakeQueueStore{
		workers: []*models.WorkerInfo{
			{ID: "w1", Hostname: "host-1", JobsHandled: 42, Status: "active"},
			{ID: "w2", Hostname: "host-2", JobsHandled: 10, Status: "active"},
		},
	}
	router := newTestRouter(newFakeJobStore(), queue)
	w := doRequest(t, router, http.MethodGet, "/api/v1/workers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	workers := resp["workers"].([]interface{})
	if len(workers) != 2 {
		t.Errorf("expected 2 workers, got %d", len(workers))
	}
}

func TestGetStats_Success(t *testing.T) {
	jobs := newFakeJobStore()
	jobs.stats = &models.JobStats{
		TotalJobs:   5,
		ActiveJobs:  3,
		PausedJobs:  1,
		TotalExecs:  20,
		SuccessExecs: 18,
		FailedExecs: 2,
	}
	queue := &fakeQueueStore{
		queue: []*models.QueuedJob{{JobID: "j1"}, {JobID: "j2"}},
	}

	router := newTestRouter(jobs, queue)
	w := doRequest(t, router, http.MethodGet, "/api/v1/stats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["queue_length"].(float64) != 2 {
		t.Errorf("expected queue_length=2, got %v", resp["queue_length"])
	}
	if resp["active_workers"].(float64) != 0 {
		t.Errorf("expected active_workers=0, got %v", resp["active_workers"])
	}
}

func TestGetStats_StoreError(t *testing.T) {
	jobs := newFakeJobStore()
	jobs.err = errors.New("pg down")
	router := newTestRouter(jobs, &fakeQueueStore{})
	w := doRequest(t, router, http.MethodGet, "/api/v1/stats", nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── Content-Type and CORS tests ───────────────────────────────────────────────

func TestCORS_OptionsRequest(t *testing.T) {
	router := newTestRouter(newFakeJobStore(), &fakeQueueStore{})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/jobs", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Should not 404 (OPTIONS is handled by CORS middleware)
	if w.Code == http.StatusNotFound {
		t.Error("OPTIONS should not 404")
	}
}

func TestCreateJob_EmptyBody(t *testing.T) {
	router := newTestRouter(newFakeJobStore(), &fakeQueueStore{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", w.Code)
	}
}

func TestCreateJob_WithRetryAndTimeout(t *testing.T) {
	jobs := newFakeJobStore()
	router := newTestRouter(jobs, &fakeQueueStore{})
	body := map[string]interface{}{
		"name":               "retry-job",
		"cron_expr":          "*/5 * * * *",
		"command":            "curl http://api/health",
		"max_retries":        5,
		"timeout_seconds":    60,
		"retry_delay_seconds": 30,
	}
	w := doRequest(t, router, http.MethodPost, "/api/v1/jobs", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}
