package classify

import (
	"context"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestFakeClassifyBilling(t *testing.T) {
	f := NewFake()
	c, err := f.Classify(context.Background(), domain.Email{
		Subject: "Question about my invoice", Body: "I was charged twice",
	})
	require.NoError(t, err)
	require.Equal(t, domain.TypeBilling, c.Type)
	require.Equal(t, domain.DeptBilling, c.Department)
	require.GreaterOrEqual(t, c.Confidence, 0.8)
}

func TestFakeClassifyCritical(t *testing.T) {
	f := NewFake()
	c, err := f.Classify(context.Background(), domain.Email{
		Subject: "URGENT: production is down", Body: "everything is failing",
	})
	require.NoError(t, err)
	require.Equal(t, domain.UrgencyCritical, c.Urgency)
}

func TestFakeClassifyAmbiguousIsLowConfidence(t *testing.T) {
	f := NewFake()
	c, err := f.Classify(context.Background(), domain.Email{
		Subject: "hello", Body: "I have a thing",
	})
	require.NoError(t, err)
	require.Less(t, c.Confidence, 0.75)
}
