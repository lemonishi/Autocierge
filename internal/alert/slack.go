package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lemonishi/autocierge/internal/domain"
)

// Slack sends an alert to a Slack incoming webhook.
type Slack struct {
	webhookURL string
	httpClient *http.Client
	baseAppURL string // e.g. "https://app.example.com" — "" means relative
}

var _ Alerter = (*Slack)(nil)

// NewSlack creates a Slack alerter. If hc is nil a 10-second client is used.
func NewSlack(webhookURL, baseAppURL string, hc *http.Client) *Slack {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Slack{webhookURL: webhookURL, baseAppURL: baseAppURL, httpClient: hc}
}

// Alert POSTs a :rotating_light: message to the Slack webhook.
func (s *Slack) Alert(ctx context.Context, t domain.Ticket) error {
	text := fmt.Sprintf(":rotating_light: *Urgent ticket* `%s` — urgency=%s type=%s. Review: %s/tickets/%s",
		t.ID, t.Urgency, t.Type, s.baseAppURL, t.ID)
	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("slack webhook status %d", resp.StatusCode)
	}
	return nil
}
