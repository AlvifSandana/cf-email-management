package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the application configuration.
type Config struct {
	Env            string
	HTTPAddr       string
	DatabaseURL    string
	MasterKey      []byte
	APIKey         string
	LogLevel       string
	RateLimit      int
	RequestTimeout time.Duration
}

// Load loads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	env := getEnv("EMS_ENV", "development")
	httpAddr := getEnv("EMS_HTTP_ADDR", ":8080")
	dbURL := getEnv("EMS_DATABASE_URL", "")
	apiKey := getEnv("EMS_API_KEY", "ems-admin-secret-key")
	logLevel := strings.ToLower(getEnv("EMS_LOG_LEVEL", "info"))

	rateLimitStr := getEnv("EMS_RATE_LIMIT", "100")
	rateLimit, err := strconv.Atoi(rateLimitStr)
	if err != nil {
		rateLimit = 100
	}

	masterKeyHex := os.Getenv("EMS_MASTER_KEY")
	var masterKey []byte
	if masterKeyHex != "" {
		decoded, err := hex.DecodeString(masterKeyHex)
		if err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("EMS_MASTER_KEY must be a 64-character hex string (32 bytes): %w", err)
		}
		masterKey = decoded
	} else {
		// In development or if not set, generate a default 32-byte key
		if env == "production" {
			return nil, fmt.Errorf("EMS_MASTER_KEY is required in production environment")
		}
		masterKey = make([]byte, 32)
		if _, err := rand.Read(masterKey); err != nil {
			return nil, fmt.Errorf("failed to generate random master key: %w", err)
		}
	}

	return &Config{
		Env:            env,
		HTTPAddr:       httpAddr,
		DatabaseURL:    dbURL,
		MasterKey:      masterKey,
		APIKey:         apiKey,
		LogLevel:       logLevel,
		RateLimit:      rateLimit,
		RequestTimeout: 30 * time.Second,
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}
