package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("PORT", "")
	t.Setenv("CONFIDENCE_THRESHOLD", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != "8080" {
		t.Fatalf("Port = %q, want 8080", c.Port)
	}
	if c.ConfidenceThreshold != 0.75 {
		t.Fatalf("ConfidenceThreshold = %v, want 0.75", c.ConfidenceThreshold)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

func TestLoadRejectsOutOfRangeThreshold(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for _, v := range []string{"-0.1", "1.5"} {
		t.Setenv("CONFIDENCE_THRESHOLD", v)
		if _, err := Load(); err == nil {
			t.Fatalf("expected error for out-of-range CONFIDENCE_THRESHOLD=%q", v)
		}
	}
}

func TestLoadDashScopeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DASHSCOPE_API_KEY", "sk-test")
	t.Setenv("DASHSCOPE_BASE_URL", "")
	t.Setenv("QWEN_MODEL", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DashScopeAPIKey != "sk-test" {
		t.Fatalf("DashScopeAPIKey = %q", c.DashScopeAPIKey)
	}
	if c.DashScopeBaseURL != "https://dashscope-intl.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("DashScopeBaseURL default = %q", c.DashScopeBaseURL)
	}
	if c.QwenModel != "qwen-max" {
		t.Fatalf("QwenModel default = %q", c.QwenModel)
	}
}

func TestLoadDashScopeOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DASHSCOPE_BASE_URL", "https://example/v1")
	t.Setenv("QWEN_MODEL", "qwen-plus")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DashScopeBaseURL != "https://example/v1" {
		t.Fatalf("DashScopeBaseURL override = %q", c.DashScopeBaseURL)
	}
	if c.QwenModel != "qwen-plus" {
		t.Fatalf("QwenModel override = %q", c.QwenModel)
	}
}

func TestIMAPSMTPSlackDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	// Ensure all IMAP/SMTP/Slack vars are unset.
	for _, k := range []string{
		"IMAP_HOST", "IMAP_PORT", "IMAP_USERNAME", "IMAP_PASSWORD", "IMAP_MAILBOX",
		"IMAP_POLL_SECONDS",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM",
		"SLACK_WEBHOOK_URL",
	} {
		t.Setenv(k, "")
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// IMAP defaults
	if c.IMAPPort != "993" {
		t.Errorf("IMAPPort = %q, want 993", c.IMAPPort)
	}
	if c.IMAPMailbox != "INBOX" {
		t.Errorf("IMAPMailbox = %q, want INBOX", c.IMAPMailbox)
	}
	if c.IMAPPollSeconds != 30 {
		t.Errorf("IMAPPollSeconds = %d, want 30", c.IMAPPollSeconds)
	}
	// IMAPEnabled false when host is empty
	if c.IMAPEnabled() {
		t.Error("IMAPEnabled() = true, want false when IMAPHost is empty")
	}

	// SMTP defaults
	if c.SMTPPort != "587" {
		t.Errorf("SMTPPort = %q, want 587", c.SMTPPort)
	}
}

func TestIMAPEnabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("IMAP_HOST", "imap.example.com")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.IMAPEnabled() {
		t.Error("IMAPEnabled() = false, want true when IMAPHost is set")
	}
}

func TestSMTPSlackRoundTrip(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_USERNAME", "user@example.com")
	t.Setenv("SMTP_PASSWORD", "s3cret")
	t.Setenv("SMTP_FROM", "support@example.com")
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.com/T123/abc")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SMTPHost != "smtp.example.com" {
		t.Errorf("SMTPHost = %q", c.SMTPHost)
	}
	if c.SMTPPort != "465" {
		t.Errorf("SMTPPort = %q", c.SMTPPort)
	}
	if c.SMTPUsername != "user@example.com" {
		t.Errorf("SMTPUsername = %q", c.SMTPUsername)
	}
	if c.SMTPPassword != "s3cret" {
		t.Errorf("SMTPPassword = %q", c.SMTPPassword)
	}
	if c.SMTPFrom != "support@example.com" {
		t.Errorf("SMTPFrom = %q", c.SMTPFrom)
	}
	if c.SlackWebhookURL != "https://hooks.slack.com/T123/abc" {
		t.Errorf("SlackWebhookURL = %q", c.SlackWebhookURL)
	}
}

func TestSMTPToRoundTrip(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SMTP_FROM", "support@example.com")
	t.Setenv("SMTP_TO", "ops@example.com")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SMTPTo != "ops@example.com" {
		t.Errorf("SMTPTo = %q, want ops@example.com", c.SMTPTo)
	}
}

func TestSMTPToDefaultsEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("SMTP_TO", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SMTPTo != "" {
		t.Errorf("SMTPTo = %q, want empty when SMTP_TO unset", c.SMTPTo)
	}
}

func TestIMAPPollSecondsRejectsNonPositive(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	for _, v := range []string{"0", "-5", "notanumber"} {
		t.Setenv("IMAP_POLL_SECONDS", v)
		c, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if c.IMAPPollSeconds != 30 {
			t.Errorf("IMAP_POLL_SECONDS=%q → IMAPPollSeconds=%d, want 30 (default)", v, c.IMAPPollSeconds)
		}
	}
}

func TestMCPServerURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("MCP_SERVER_URL", "http://127.0.0.1:8090/mcp")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.MCPServerURL != "http://127.0.0.1:8090/mcp" {
		t.Errorf("MCPServerURL = %q", c.MCPServerURL)
	}
}

func TestMCPServerURLDefaultsEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("MCP_SERVER_URL", "")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.MCPServerURL != "" {
		t.Errorf("MCPServerURL = %q, want empty when unset", c.MCPServerURL)
	}
}
