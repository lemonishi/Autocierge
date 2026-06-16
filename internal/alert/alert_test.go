package alert

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRecordingAlerter(t *testing.T) {
	a := NewRecording()
	tk := domain.Ticket{ID: uuid.New(), Urgency: domain.UrgencyCritical}
	require.NoError(t, a.Alert(context.Background(), tk))
	require.Len(t, a.Sent, 1)
	require.Equal(t, tk.ID, a.Sent[0].ID)
}
