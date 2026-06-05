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
