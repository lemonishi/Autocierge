package alert

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failing is a stub Alerter that always returns an error.
type failing struct{}

func (f *failing) Alert(_ context.Context, _ domain.Ticket) error {
	return errors.New("alerter failed")
}

func TestMultiCallsAllChildren(t *testing.T) {
	rec1 := NewRecording()
	rec2 := NewRecording()
	fail := &failing{}

	tk := domain.Ticket{ID: uuid.New(), Urgency: domain.UrgencyCritical}

	m := NewMulti(rec1, fail, rec2)
	err := m.Alert(context.Background(), tk)

	// best-effort: Multi always returns nil even if a child fails
	require.NoError(t, err)

	// all children (including those after the failing one) were invoked
	assert.Len(t, rec1.Sent, 1, "rec1 should have received the alert")
	assert.Len(t, rec2.Sent, 1, "rec2 should have received the alert (runs after failing)")
	assert.Equal(t, tk.ID, rec1.Sent[0].ID)
	assert.Equal(t, tk.ID, rec2.Sent[0].ID)
}

func TestMultiEmptyIsNoop(t *testing.T) {
	m := NewMulti()
	tk := domain.Ticket{ID: uuid.New()}
	require.NoError(t, m.Alert(context.Background(), tk))
}
