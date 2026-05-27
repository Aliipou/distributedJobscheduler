package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Scheduler metrics
var (
	// JobsEnqueued counts the total number of jobs successfully pushed to the queue.
	JobsEnqueued = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jobscheduler",
		Subsystem: "scheduler",
		Name:      "jobs_enqueued_total",
		Help:      "Total number of jobs enqueued for execution.",
	}, []string{"job_name"})

	// ScheduleErrors counts failures that occurred while scheduling a job.
	ScheduleErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jobscheduler",
		Subsystem: "scheduler",
		Name:      "schedule_errors_total",
		Help:      "Total number of errors encountered while scheduling jobs.",
	}, []string{"job_name"})
)

// Worker metrics
var (
	// JobsExecuted counts the total number of jobs executed by workers.
	JobsExecuted = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jobscheduler",
		Subsystem: "worker",
		Name:      "jobs_executed_total",
		Help:      "Total number of jobs executed (success + failure).",
	}, []string{"job_id", "status"})

	// JobsFailed counts the total number of failed job executions.
	// A job counts as failed when it exhausts all retries and is moved to the
	// dead-letter queue, or when a non-retriable error occurs.
	JobsFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jobscheduler",
		Subsystem: "worker",
		Name:      "jobs_failed_total",
		Help:      "Total number of jobs that failed (dead-lettered or terminal error).",
	}, []string{"job_id"})

	// JobDuration tracks the wall-clock time spent executing each job.
	JobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "jobscheduler",
		Subsystem: "worker",
		Name:      "job_duration_seconds",
		Help:      "Histogram of job execution durations in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"job_id"})
)
