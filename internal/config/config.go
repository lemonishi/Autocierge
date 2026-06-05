// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Port                string
	DatabaseURL         string
	ConfidenceThreshold float64
	DashScopeAPIKey     string
	DashScopeBaseURL    string
	QwenModel           string
}

func Load() (Config, error) {
	c := Config{
		Port:                getenv("PORT", "8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		ConfidenceThreshold: 0.75,
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if v := os.Getenv("CONFIDENCE_THRESHOLD"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return Config{}, errors.New("CONFIDENCE_THRESHOLD must be a float")
		}
		if f < 0 || f > 1 {
			return Config{}, errors.New("CONFIDENCE_THRESHOLD must be between 0 and 1")
		}
		c.ConfidenceThreshold = f
	}
	c.DashScopeAPIKey = os.Getenv("DASHSCOPE_API_KEY")
	c.DashScopeBaseURL = getenv("DASHSCOPE_BASE_URL", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1")
	c.QwenModel = getenv("QWEN_MODEL", "qwen-max")
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
