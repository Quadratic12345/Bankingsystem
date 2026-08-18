package config

import (
	"os"
	"time"
)
type Config struct {
	DBUrl          string
	JWTSecret      string
	JWTTTL         time.Duration
	ServerPort     string
	MaxTxRetries   int
	TxRetryBaseDur time.Duration
}

func Load() *Config {
	return &Config{
		DBUrl:          getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/banking?sslmode=disable"),
		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production"),
		JWTTTL:         15 * time.Minute,
		ServerPort:     getEnv("PORT", "8080"),
		MaxTxRetries:   5,
		TxRetryBaseDur: 20 * time.Millisecond,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}