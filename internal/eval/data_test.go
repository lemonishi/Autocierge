package eval_test

import (
	"os"
	"path/filepath"
	"testing"

	dom "github.com/lemonishi/autocierge/internal/domain"
	"github.com/lemonishi/autocierge/internal/eval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGold(t *testing.T) {
	golds, err := eval.LoadGold("../../eval/gold.jsonl")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(golds), 30)

	for _, g := range golds {
		assert.NotEmpty(t, g.ID, "id must be set")
		assert.True(t, dom.ValidUrgency(g.GoldUrgency), "bad urgency %q in %s", g.GoldUrgency, g.ID)
		assert.True(t, dom.ValidType(g.GoldType), "bad type %q in %s", g.GoldType, g.ID)
	}

	seen := map[string]bool{}
	for _, g := range golds {
		assert.False(t, seen[g.ID], "duplicate id %s", g.ID)
		seen[g.ID] = true
	}
}

func TestPredictionsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rec.json")
	in := map[string]eval.Prediction{
		"g01": {ID: "g01", Urgency: "normal", Type: "billing", Department: "billing", Confidence: 0.9},
	}
	require.NoError(t, eval.SavePredictions(path, in))

	out, err := eval.LoadPredictions(path)
	require.NoError(t, err)
	assert.Equal(t, in, out)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "g01")
}

func TestLabelsMatchDomain(t *testing.T) {
	for _, u := range eval.UrgencyLabels {
		assert.True(t, dom.ValidUrgency(dom.Urgency(u)), "UrgencyLabels has unknown value %q", u)
	}
	for _, tp := range eval.TypeLabels {
		assert.True(t, dom.ValidType(dom.TicketType(tp)), "TypeLabels has unknown value %q", tp)
	}
}

func TestPredictionCorrect(t *testing.T) {
	g := eval.GoldEmail{ID: "x", GoldUrgency: "high", GoldType: "billing"}
	assert.True(t, eval.Prediction{Urgency: "high", Type: "billing"}.Correct(g))
	assert.False(t, eval.Prediction{Urgency: "low", Type: "billing"}.Correct(g), "wrong urgency")
	assert.False(t, eval.Prediction{Urgency: "high", Type: "technical"}.Correct(g), "wrong type")
}
