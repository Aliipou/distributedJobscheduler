package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/models"
	"github.com/aliipou/distributed-job-scheduler/internal/scheduler"
	"github.com/aliipou/distributed-job-scheduler/internal/store"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	pg    store.JobStore
	redis store.QueueStore
	log   *zap.Logger
}

func NewHandler(pg store.JobStore, r store.QueueStore, log *zap.Logger) *Handler {
	return &Handler{pg: pg, redis: r, log: log}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.Use(CORS())
	v1 := r.Group("/api/v1")
	{
		jobs := v1.Group("/jobs")
		jobs.POST("", h.CreateJob)
		jobs.GET("", h.ListJobs)
		jobs.GET("/:id", h.GetJob)
		jobs.DELETE("/:id", h.DeleteJob)
		jobs.POST("/:id/trigger", h.TriggerJob)
		jobs.POST("/:id/pause", h.PauseJob)
		jobs.POST("/:id/resume", h.ResumeJob)
		jobs.GET("/:id/executions", h.ListJobExecutions)

		v1.GET("/executions", h.ListExecutions)
		v1.GET("/workers", h.ListWorkers)
		v1.GET("/stats", h.GetStats)
	}
	r.Static("/web", "./web")
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/web/index.html") })
}

func (h *Handler) CreateJob(c *gin.Context) {
	var req models.CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := scheduler.ValidateCron(req.CronExpr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cron expression: " + err.Error()})
		return
	}
	nextRun, _ := scheduler.NextRun(req.CronExpr, time.Now())
	job, err := h.pg.CreateJob(c.Request.Context(), &req, nextRun)
	if err != nil {
		h.log.Error("create job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create job"})
		return
	}
	c.JSON(http.StatusCreated, job)
}

func (h *Handler) ListJobs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	jobs, total, err := h.pg.ListJobs(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": jobs, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) GetJob(c *gin.Context) {
	job, err := h.pg.GetJob(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *Handler) DeleteJob(c *gin.Context) {
	if err := h.pg.UpdateJobStatus(c.Request.Context(), c.Param("id"), models.JobStatusDeleted); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "job deleted"})
}

func (h *Handler) TriggerJob(c *gin.Context) {
	job, err := h.pg.GetJob(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	queued := &models.QueuedJob{
		JobID:      job.ID,
		Command:    job.Command,
		Payload:    string(job.Payload),
		Attempt:    1,
		MaxRetries: job.MaxRetries,
		TimeoutSec: job.TimeoutSeconds,
	}
	if err := h.redis.EnqueueJob(c.Request.Context(), queued); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "job triggered"})
}

func (h *Handler) PauseJob(c *gin.Context) {
	if err := h.pg.UpdateJobStatus(c.Request.Context(), c.Param("id"), models.JobStatusPaused); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "job paused"})
}

func (h *Handler) ResumeJob(c *gin.Context) {
	if err := h.pg.UpdateJobStatus(c.Request.Context(), c.Param("id"), models.JobStatusActive); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "job resumed"})
}

func (h *Handler) ListJobExecutions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	execs, err := h.pg.ListExecutions(c.Request.Context(), c.Param("id"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"executions": execs})
}

func (h *Handler) ListExecutions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	execs, err := h.pg.ListExecutions(c.Request.Context(), "", limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"executions": execs})
}

func (h *Handler) ListWorkers(c *gin.Context) {
	workers, err := h.redis.GetWorkers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"workers": workers})
}

func (h *Handler) GetStats(c *gin.Context) {
	stats, err := h.pg.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	qLen, _ := h.redis.QueueLength(c.Request.Context())
	workers, _ := h.redis.GetWorkers(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"jobs":         stats,
		"queue_length": qLen,
		"active_workers": len(workers),
	})
}
