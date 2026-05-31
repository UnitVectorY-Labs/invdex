package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Server
	Port int

	// Database
	DatabaseURL string

	// Storage
	StorageBackend string // "s3" or "gcs"
	StorageBucket  string

	// S3-specific
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string

	// GCS-specific
	GCSCredentialsFile string

	// LLM
	LLMProvider string // "openai" compatible
	LLMEndpoint string
	LLMAPIKey   string
	LLMModel    string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	port := getEnvInt("PORT", 8080)

	dbURL := getEnv("DATABASE_URL", "")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	cfg := &Config{
		Port:               port,
		DatabaseURL:        dbURL,
		StorageBackend:     getEnv("STORAGE_BACKEND", "s3"),
		StorageBucket:      getEnv("STORAGE_BUCKET", "invdex"),
		S3Endpoint:         getEnv("S3_ENDPOINT", ""),
		S3Region:           getEnv("S3_REGION", "us-east-1"),
		S3AccessKey:        getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:        getEnv("S3_SECRET_KEY", ""),
		GCSCredentialsFile: getEnv("GCS_CREDENTIALS_FILE", ""),
		LLMProvider:        getEnv("LLM_PROVIDER", "openai"),
		LLMEndpoint:        getEnv("LLM_ENDPOINT", "https://api.openai.com/v1"),
		LLMAPIKey:          getEnv("LLM_API_KEY", ""),
		LLMModel:           getEnv("LLM_MODEL", "gpt-4o"),
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
