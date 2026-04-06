package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr             string
	DatabaseURL          string
	DefaultReviewerID    string
	ReminderAfter        time.Duration
	ExecutionRetryBase   time.Duration
	ExecutionMaxAttempts int
	WorkerPollInterval   time.Duration
	WorkerBatchSize      int
	OTel                 OTelConfig
}

type OTelConfig struct {
	Enabled          bool
	ServiceName      string
	ExporterEndpoint string
	Insecure         bool
	SampleRatio      float64
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:             getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		DefaultReviewerID:    getEnv("DEFAULT_REVIEWER_ID", "reviewer-1"),
		ReminderAfter:        mustDuration("REMINDER_AFTER", 30*time.Minute),
		ExecutionRetryBase:   mustDuration("EXECUTION_RETRY_BASE", 15*time.Second),
		ExecutionMaxAttempts: mustInt("EXECUTION_MAX_ATTEMPTS", 3),
		WorkerPollInterval:   mustDuration("WORKER_POLL_INTERVAL", 10*time.Second),
		WorkerBatchSize:      mustInt("WORKER_BATCH_SIZE", 25),
		OTel: OTelConfig{
			Enabled:          mustBool("OTEL_ENABLED", false),
			ServiceName:      getEnv("OTEL_SERVICE_NAME", "review-workflow"),
			ExporterEndpoint: strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
			Insecure:         mustBool("OTEL_EXPORTER_OTLP_INSECURE", false),
			SampleRatio:      mustFloat("OTEL_SAMPLE_RATIO", 1.0),
		},
	}

	if cfg.DefaultReviewerID == "" {
		return Config{}, errors.New("DEFAULT_REVIEWER_ID is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.ExecutionMaxAttempts <= 0 {
		return Config{}, fmt.Errorf("EXECUTION_MAX_ATTEMPTS must be positive")
	}
	if cfg.OTel.ServiceName == "" {
		return Config{}, errors.New("OTEL_SERVICE_NAME must not be empty")
	}
	if cfg.OTel.SampleRatio < 0 || cfg.OTel.SampleRatio > 1 {
		return Config{}, fmt.Errorf("OTEL_SAMPLE_RATIO must be between 0 and 1")
	}
	if cfg.OTel.Enabled && cfg.OTel.ExporterEndpoint == "" {
		return Config{}, errors.New("OTEL_EXPORTER_OTLP_ENDPOINT is required when OTEL_ENABLED=true")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func mustDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func mustInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func mustBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func mustFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
