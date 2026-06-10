package alert

import (
	"context"
	"fmt"
	"net/smtp"
	"time"

	"github.com/lemonishi/supportsentinel/internal/domain"
)

// sendFunc matches the signature of smtp.SendMail, allowing injection in tests.
type sendFunc func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error

// Email sends an RFC822 alert message via SMTP.
type Email struct {
	host, port, username, password, from, to, baseAppURL string
	send                                                  sendFunc // defaults to smtp.SendMail
}

var _ Alerter = (*Email)(nil)

// NewEmail constructs an Email alerter. The actual network send defaults to smtp.SendMail.
func NewEmail(host, port, user, pass, from, to, baseAppURL string) *Email {
	return &Email{
		host:       host,
		port:       port,
		username:   user,
		password:   pass,
		from:       from,
		to:         to,
		baseAppURL: baseAppURL,
		send:       smtp.SendMail,
	}
}

// Alert builds an RFC822 message and sends it via SMTP.
// ctx is accepted for interface symmetry; smtp.SendMail is not ctx-aware (acceptable for best-effort alerts).
func (e *Email) Alert(_ context.Context, t domain.Ticket) error {
	subject := fmt.Sprintf("[URGENT] support ticket %s (%s)", t.ID, t.Urgency)
	body := fmt.Sprintf("Urgent ticket needs review.\r\nID: %s\r\nUrgency: %s\r\nType: %s\r\nReview: %s/tickets/%s\r\n",
		t.ID, t.Urgency, t.Type, e.baseAppURL, t.ID)
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nDate: %s\r\nSubject: %s\r\n\r\n%s",
		e.from, e.to, time.Now().UTC().Format(time.RFC1123Z), subject, body))
	auth := smtp.PlainAuth("", e.username, e.password, e.host)
	return e.send(e.host+":"+e.port, auth, e.from, []string{e.to}, msg)
}
