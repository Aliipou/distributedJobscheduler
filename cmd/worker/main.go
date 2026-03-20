package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aliipou/distributed-job-scheduler/internal/config"
	"github.com/aliipou/distributed-job-scheduler/internal/store"
	"github.com/aliipou/distributed-job-scheduler/internal/worker"
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

	pg, err := store.NewPostgresStore(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer pg.Close()

	redis, err := store.NewRedisStore(cfg.RedisURL)
	if err != nil {
		log.Fatal("connect redis", zap.Error(err))
	}
	defer redis.Close()

	pool := worker.NewPool(cfg.WorkerConcurrency, cfg.WorkerID, pg, redis, log)
	log.Info("worker pool starting",
		zap.Int("concurrency", cfg.WorkerConcurrency),
		zap.String("worker_id_prefix", cfg.WorkerID),
	)
	pool.Run(ctx)
	log.Info("worker pool stopped")
}
