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

	// IMAP ingestion (optional — features activate only when IMAPHost is set)
	IMAPHost        string
	IMAPPort        string // default "993"
	IMAPUsername    string
	IMAPPassword    string
	IMAPMailbox     string // default "INBOX"
	IMAPPollSeconds int    // default 30

	// Alerting (optional)
	SMTPHost        string
	SMTPPort        string // default "587"
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	SlackWebhookURL string
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

	// IMAP ingestion (all optional)
	c.IMAPHost = os.Getenv("IMAP_HOST")
	c.IMAPPort = getenv("IMAP_PORT", "993")
	c.IMAPUsername = os.Getenv("IMAP_USERNAME")
	c.IMAPPassword = os.Getenv("IMAP_PASSWORD")
	c.IMAPMailbox = getenv("IMAP_MAILBOX", "INBOX")
	c.IMAPPollSeconds = 30
	if v := os.Getenv("IMAP_POLL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.IMAPPollSeconds = n
		}
	}

	// Alerting (all optional)
	c.SMTPHost = os.Getenv("SMTP_HOST")
	c.SMTPPort = getenv("SMTP_PORT", "587")
	c.SMTPUsername = os.Getenv("SMTP_USERNAME")
	c.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	c.SMTPFrom = os.Getenv("SMTP_FROM")
	c.SlackWebhookURL = os.Getenv("SLACK_WEBHOOK_URL")

	return c, nil
}

// IMAPEnabled reports whether IMAP ingestion is configured.
func (c Config) IMAPEnabled() bool { return c.IMAPHost != "" }

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
