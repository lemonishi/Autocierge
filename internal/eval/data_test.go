package eval_test

import (
	"os"
	"path/filepath"
	"testing"

	dom "github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/lemonishi/supportsentinel/internal/eval"
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
