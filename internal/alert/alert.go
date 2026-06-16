// Package alert sends best-effort, one-way urgent notifications. Real Slack/email
// delivery arrives in Plan 5; alerting must NEVER block or fail the pipeline.
package alert

import (
	"context"
	"log"

	"github.com/lemonishi/autocierge/internal/domain"
)

type Alerter interface {
	Alert(ctx context.Context, t domain.Ticket) error
}

var (
	_ Alerter = (*Log)(nil)
	_ Alerter = (*Recording)(nil)
)

// Log writes alerts to stdout (default impl until Plan 5).
type Log struct{}

func NewLog() *Log { return &Log{} }

func (l *Log) Alert(_ context.Context, t domain.Ticket) error {
	log.Printf("[ALERT] urgent ticket %s (urgency=%s) needs review", t.ID, t.Urgency)
	return nil
}

// Recording captures alerts for tests.
type Recording struct{ Sent []domain.Ticket }

func NewRecording() *Recording { return &Recording{} }

func (r *Recording) Alert(_ context.Context, t domain.Ticket) error {
	r.Sent = append(r.Sent, t)
	return nil
}
