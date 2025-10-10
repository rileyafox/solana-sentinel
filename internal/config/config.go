
package config

import (
	"os"
)

type Config struct {
	SolanaHTTP string
	SolanaWS   string
	DatabaseURL string
	RedisAddr  string
	LogLevel   string
}

func Load() Config {
	cfg := Config{
		SolanaHTTP: getenv("SOLANA_HTTP_URL", "https://api.devnet.solana.com"),
		SolanaWS:   getenv("SOLANA_WS_URL", "wss://api.devnet.solana.com"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5433/sentinel?sslmode=disable"),
		RedisAddr:  getenv("REDIS_ADDR", "localhost:6379"),
		LogLevel:   getenv("LOG_LEVEL", "info"),
	}
	return cfg
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" { return v }
	return def
}
