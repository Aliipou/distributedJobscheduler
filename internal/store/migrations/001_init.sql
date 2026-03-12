CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    cron_expr VARCHAR(100) NOT NULL,
    command TEXT NOT NULL,
    payload JSONB DEFAULT '{}',
    max_retries INT DEFAULT 3,
    retry_delay_seconds INT DEFAULT 60,
    timeout_seconds INT DEFAULT 300,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID REFERENCES jobs(id) ON DELETE CASCADE,
    worker_id VARCHAR(100),
    status VARCHAR(50) DEFAULT 'pending',
    attempt INT DEFAULT 1,
    output TEXT DEFAULT '',
    error TEXT DEFAULT '',
    started_at TIMESTAMPTZ DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_jobs_next_run ON jobs(next_run_at, status);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_executions_job ON executions(job_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_executions_status ON executions(status);
CREATE INDEX IF NOT EXISTS idx_executions_started ON executions(started_at DESC);
