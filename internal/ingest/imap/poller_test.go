package imap

import (
	"context"
	"testing"
	"time"

	"github.com/lemonishi/autocierge/internal/config"
	"github.com/lemonishi/autocierge/internal/domain"
)

// fakeIngestor is a compile-time and runtime check that an Ingestor can be
// satisfied without importing the orchestrator. It records the last call.
type fakeIngestor struct {
	calls   int
	lastSrc string
	lastEml domain.Email
}

func (f *fakeIngestor) Ingest(_ context.Context, source string, e domain.Email) (domain.Ticket, error) {
	f.calls++
	f.lastSrc = source
	f.lastEml = e
	return domain.Ticket{}, nil
}

// Compile-time assertion: the seam is satisfiable by a plain struct.
var _ Ingestor = (*fakeIngestor)(nil)

func TestNewSetsIntervalFromConfig(t *testing.T) {
	cfg := config.Config{IMAPPollSeconds: 45}
	p := New(cfg, &fakeIngestor{})
	if p.every != 45*time.Second {
		t.Fatalf("every = %v, want %v", p.every, 45*time.Second)
	}
	if p.ing == nil {
		t.Fatal("ingestor not set")
	}
	if p.cfg.IMAPPollSeconds != 45 {
		t.Fatalf("cfg not stored: got %d", p.cfg.IMAPPollSeconds)
	}
}
