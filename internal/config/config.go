package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	PostgresURL       string `mapstructure:"POSTGRES_URL"`
	RedisURL          string `mapstructure:"REDIS_URL"`
	HTTPPort          int    `mapstructure:"HTTP_PORT"`
	WorkerConcurrency int    `mapstructure:"WORKER_CONCURRENCY"`
	WorkerID          string `mapstructure:"WORKER_ID"`
	LogLevel          string `mapstructure:"LOG_LEVEL"`
	ScheduleInterval  int    `mapstructure:"SCHEDULE_INTERVAL_SECONDS"`
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	viper.SetDefault("HTTP_PORT", 8080)
	viper.SetDefault("WORKER_CONCURRENCY", 5)
	viper.SetDefault("WORKER_ID", "worker-1")
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("SCHEDULE_INTERVAL_SECONDS", 10)
	viper.SetDefault("POSTGRES_URL", "postgres://scheduler:password@localhost:5432/scheduler_db?sslmode=disable")
	viper.SetDefault("REDIS_URL", "redis://localhost:6379")

	_ = viper.ReadInConfig()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
