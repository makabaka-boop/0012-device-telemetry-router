// Package config parses environment variables into a typed Config.
package config

import (
	"fmt"
	"os"
	"time"
)

// WorkerConfig holds background retry worker tuning.
type WorkerConfig struct {
	Enabled    bool
	Interval   time.Duration
	BatchSize  int
	MaxAttempt int
	BaseDelay  time.Duration
}

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Port            string
	DatabaseURL     string
	DedupWindow     time.Duration
	ShutdownTimeout time.Duration
	APIKeys         string
	Worker          WorkerConfig
}

// Load reads the configuration from the process environment, applying
// defaults where variables are absent.
func Load() (*Config, error) {
	cfg := &Config{
		Port:            getenv("PORT", "8080"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/telemetry?sslmode=disable"),
		DedupWindow:     getDuration("DEDUP_WINDOW", 24*time.Hour),
		ShutdownTimeout: getDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		APIKeys:         os.Getenv("API_KEYS"),
		Worker: WorkerConfig{
			Enabled:    getBool("WORKER_ENABLED", true),
			Interval:   getDuration("WORKER_INTERVAL", 2*time.Second),
			BatchSize:  getInt("WORKER_BATCH_SIZE", 100),
			MaxAttempt: getInt("WORKER_MAX_ATTEMPT", 5),
			BaseDelay:  getDuration("WORKER_BASE_DELAY", time.Second),
		},
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL must not be empty")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	switch v {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	}
	return def
}

func getInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n := def
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}
