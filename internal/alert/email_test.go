package alert

import (
	"context"
	"net/smtp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailAlert(t *testing.T) {
	const (
		host       = "smtp.example.com"
		port       = "587"
		username   = "user@example.com"
		password   = "secret"
		from       = "alerts@example.com"
		to         = "ops@example.com"
		baseAppURL = "https://app.example.com"
	)

	tk := domain.Ticket{
		ID:      uuid.New(),
		Urgency: domain.UrgencyCritical,
		Type:    domain.TypeBilling,
	}

	var (
		capturedAddr string
		capturedFrom string
		capturedTo   []string
		capturedMsg  []byte
		callCount    int
	)

	fakeSend := func(addr string, auth smtp.Auth, f string, t []string, msg []byte) error {
		capturedAddr = addr
		capturedFrom = f
		capturedTo = t
		capturedMsg = msg
		callCount++
		return nil
	}

	e := NewEmail(host, port, username, password, from, to, baseAppURL)
	e.send = fakeSend // inject via unexported field (same package)

	err := e.Alert(context.Background(), tk)
	require.NoError(t, err)

	// send called exactly once
	assert.Equal(t, 1, callCount)

	// addr is host:port
	assert.Equal(t, host+":"+port, capturedAddr)

	// from and to are correct
	assert.Equal(t, from, capturedFrom)
	require.Len(t, capturedTo, 1)
	assert.Equal(t, to, capturedTo[0])

	msgStr := string(capturedMsg)

	// RFC822 headers
	assert.Contains(t, msgStr, "From: "+from)
	assert.Contains(t, msgStr, "To: "+to)
	assert.Contains(t, msgStr, "[URGENT] support ticket "+tk.ID.String())

	// blank line separating headers from body
	assert.Contains(t, msgStr, "\r\n\r\n")

	// body mentions urgency, type, and link
	assert.Contains(t, msgStr, string(tk.Urgency))
	assert.Contains(t, msgStr, string(tk.Type))
	assert.Contains(t, msgStr, baseAppURL+"/tickets/"+tk.ID.String())

	// password must NOT appear in the message
	assert.False(t, strings.Contains(msgStr, password), "password must not appear in message")
}
