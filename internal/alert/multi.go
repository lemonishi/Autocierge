package alert

import (
	"context"
	"log"

	"github.com/lemonishi/autocierge/internal/config"
	"github.com/lemonishi/autocierge/internal/domain"
)

// Multi fans out an Alert call to every child alerter. Failures are logged
// and never propagated — alerting must never block the ticket pipeline.
type Multi struct{ alerters []Alerter }

var _ Alerter = (*Multi)(nil)

// NewMulti constructs a Multi alerter from the provided children.
func NewMulti(a ...Alerter) *Multi { return &Multi{alerters: a} }

// Alert calls each child in order. If a child returns an error, the failure is
// logged (with the child's type and ticket ID) and the remaining children still
// run. Multi.Alert always returns nil (best-effort semantics).
func (m *Multi) Alert(ctx context.Context, t domain.Ticket) error {
	for _, a := range m.alerters {
		if err := a.Alert(ctx, t); err != nil {
			log.Printf("alert: %T failed for ticket %s: %v", a, t.ID, err)
		}
	}
	return nil // best-effort: never propagate errors
}

// FromConfig builds the alerter chain from runtime configuration.
// Always includes a Log alerter; adds Slack when SlackWebhookURL is set;
// adds Email when SMTPHost and SMTPFrom are both set.
// The recipient is SMTPTo when non-empty, else falls back to SMTPFrom.
func FromConfig(c config.Config) Alerter {
	as := []Alerter{NewLog()}
	if c.SlackWebhookURL != "" {
		as = append(as, NewSlack(c.SlackWebhookURL, "", nil))
	}
	if c.SMTPHost != "" && c.SMTPFrom != "" {
		to := c.SMTPTo
		if to == "" {
			to = c.SMTPFrom
		}
		as = append(as, NewEmail(c.SMTPHost, c.SMTPPort, c.SMTPUsername, c.SMTPPassword, c.SMTPFrom, to, ""))
	}
	return NewMulti(as...)
}
