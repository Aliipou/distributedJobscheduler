package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aliipou/distributed-job-scheduler/internal/api"
	"github.com/aliipou/distributed-job-scheduler/internal/config"
	"github.com/aliipou/distributed-job-scheduler/internal/scheduler"
	"github.com/aliipou/distributed-job-scheduler/internal/store"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	log, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// PostgreSQL
	pg, err := store.NewPostgresStore(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer pg.Close()

	if err := pg.Migrate(ctx); err != nil {
		log.Fatal("migrate", zap.Error(err))
	}

	// Redis
	redis, err := store.NewRedisStore(cfg.RedisURL)
	if err != nil {
		log.Fatal("connect redis", zap.Error(err))
	}
	defer redis.Close()

	// Start scheduler
	sched := scheduler.New(pg, redis, cfg.ScheduleInterval, log)
	go sched.Run(ctx)

	// HTTP API
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(api.Logger(log), api.CORS(), gin.Recovery())

	handler := api.NewHandler(pg, redis, log)
	handler.RegisterRoutes(engine)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: engine,
	}

	go func() {
		log.Info("API server listening", zap.Int("port", cfg.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("listen", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	log.Info("server stopped")
}
