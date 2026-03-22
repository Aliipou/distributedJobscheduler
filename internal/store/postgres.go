package store

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var migrationSQL string

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, connStr string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, migrationSQL)
	return err
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// ── Jobs ──────────────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateJob(ctx context.Context, req *models.CreateJobRequest, nextRun time.Time) (*models.Job, error) {
	job := &models.Job{
		ID:                uuid.New().String(),
		Name:              req.Name,
		CronExpr:          req.CronExpr,
		Command:           req.Command,
		Payload:           req.Payload,
		MaxRetries:        req.MaxRetries,
		RetryDelaySeconds: req.RetryDelaySeconds,
		TimeoutSeconds:    req.TimeoutSeconds,
		Status:            models.JobStatusActive,
		NextRunAt:         &nextRun,
	}
	if job.MaxRetries == 0 {
		job.MaxRetries = 3
	}
	if job.RetryDelaySeconds == 0 {
		job.RetryDelaySeconds = 60
	}
	if job.TimeoutSeconds == 0 {
		job.TimeoutSeconds = 300
	}
	if job.Payload == nil {
		job.Payload = []byte("{}")
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (id, name, cron_expr, command, payload, max_retries, retry_delay_seconds, timeout_seconds, status, next_run_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		job.ID, job.Name, job.CronExpr, job.Command, job.Payload,
		job.MaxRetries, job.RetryDelaySeconds, job.TimeoutSeconds, job.Status, job.NextRunAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}
	return job, nil
}

func (s *PostgresStore) GetJob(ctx context.Context, id string) (*models.Job, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, cron_expr, command, payload, max_retries, retry_delay_seconds,
		       timeout_seconds, status, created_at, updated_at, next_run_at, last_run_at
		FROM jobs WHERE id = $1 AND status != 'deleted'`, id)
	return scanJob(row)
}

func (s *PostgresStore) ListJobs(ctx context.Context, limit, offset int) ([]*models.Job, int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE status != 'deleted'`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, name, cron_expr, command, payload, max_retries, retry_delay_seconds,
		       timeout_seconds, status, created_at, updated_at, next_run_at, last_run_at
		FROM jobs WHERE status != 'deleted'
		ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []*models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	return jobs, total, nil
}

func (s *PostgresStore) UpdateJobStatus(ctx context.Context, id string, status models.JobStatus) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status=$1, updated_at=NOW() WHERE id=$2`, status, id)
	return err
}

func (s *PostgresStore) UpdateNextRun(ctx context.Context, id string, nextRun time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET next_run_at=$1, last_run_at=NOW(), updated_at=NOW() WHERE id=$2`, nextRun, id)
	return err
}

func (s *PostgresStore) GetDueJobs(ctx context.Context, now time.Time) ([]*models.Job, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, cron_expr, command, payload, max_retries, retry_delay_seconds,
		       timeout_seconds, status, created_at, updated_at, next_run_at, last_run_at
		FROM jobs WHERE status = 'active' AND next_run_at <= $1
		ORDER BY next_run_at ASC`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// ── Executions ────────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateExecution(ctx context.Context, jobID, workerID string, attempt int) (*models.Execution, error) {
	exec := &models.Execution{
		ID:       uuid.New().String(),
		JobID:    jobID,
		WorkerID: workerID,
		Status:   models.ExecStatusPending,
		Attempt:  attempt,
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO executions (id, job_id, worker_id, status, attempt)
		VALUES ($1,$2,$3,$4,$5)`,
		exec.ID, exec.JobID, exec.WorkerID, exec.Status, exec.Attempt,
	)
	return exec, err
}

func (s *PostgresStore) UpdateExecution(ctx context.Context, id string, status models.ExecStatus, output, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE executions SET status=$1, output=$2, error=$3, finished_at=NOW()
		WHERE id=$4`, status, output, errMsg, id)
	return err
}

func (s *PostgresStore) UpdateExecutionStatus(ctx context.Context, id string, status models.ExecStatus) error {
	_, err := s.pool.Exec(ctx, `UPDATE executions SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (s *PostgresStore) ListExecutions(ctx context.Context, jobID string, limit int) ([]*models.Execution, error) {
	query := `SELECT id, job_id, worker_id, status, attempt, output, error, started_at, finished_at
		FROM executions`
	args := []interface{}{limit}
	if jobID != "" {
		query += " WHERE job_id=$2 ORDER BY started_at DESC LIMIT $1"
		args = append(args, jobID)
	} else {
		query += " ORDER BY started_at DESC LIMIT $1"
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []*models.Execution
	for rows.Next() {
		e := &models.Execution{}
		if err := rows.Scan(&e.ID, &e.JobID, &e.WorkerID, &e.Status, &e.Attempt, &e.Output, &e.Error, &e.StartedAt, &e.FinishedAt); err != nil {
			return nil, err
		}
		execs = append(execs, e)
	}
	return execs, nil
}

func (s *PostgresStore) GetStats(ctx context.Context) (*models.JobStats, error) {
	stats := &models.JobStats{}
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status != 'deleted') as total,
			COUNT(*) FILTER (WHERE status = 'active') as active,
			COUNT(*) FILTER (WHERE status = 'paused') as paused
		FROM jobs`).Scan(&stats.TotalJobs, &stats.ActiveJobs, &stats.PausedJobs)
	if err != nil {
		return nil, err
	}
	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'success') as success,
			COUNT(*) FILTER (WHERE status = 'failed') as failed,
			COUNT(*) FILTER (WHERE status = 'dead_letter') as dead
		FROM executions`).Scan(&stats.TotalExecs, &stats.SuccessExecs, &stats.FailedExecs, &stats.DeadLetters)
	return stats, err
}

// ── helpers ───────────────────────────────────────────────────────────────────

type scannable interface {
	Scan(dest ...any) error
}

func scanJob(row scannable) (*models.Job, error) {
	j := &models.Job{}
	err := row.Scan(
		&j.ID, &j.Name, &j.CronExpr, &j.Command, &j.Payload,
		&j.MaxRetries, &j.RetryDelaySeconds, &j.TimeoutSeconds,
		&j.Status, &j.CreatedAt, &j.UpdatedAt, &j.NextRunAt, &j.LastRunAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}
	return j, nil
}
