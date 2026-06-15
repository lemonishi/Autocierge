package eval_test

import (
	"strings"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/eval"
	"github.com/stretchr/testify/assert"
)

func TestRenderReport(t *testing.T) {
	golds := []eval.GoldEmail{
		{ID: "1", GoldUrgency: "normal", GoldType: "billing"},
		{ID: "2", GoldUrgency: "critical", GoldType: "technical"},
	}
	preds := map[string]eval.Prediction{
		"1": {ID: "1", Urgency: "normal", Type: "billing", Department: "billing", Confidence: 0.9},
		"2": {ID: "2", Urgency: "critical", Type: "technical", Department: "engineering", Confidence: 0.95},
	}

	out := eval.Render(golds, preds, 0.90)

	assert.Contains(t, out, "URGENCY")
	assert.Contains(t, out, "TYPE")
	assert.Contains(t, out, "Confusion")
	assert.Contains(t, out, "Calibration")
	assert.Contains(t, out, "Recommended CONFIDENCE_THRESHOLD")
	assert.Contains(t, out, "Overall accuracy")
	assert.True(t, strings.Contains(out, "100.0%") || strings.Contains(out, "1.000"))
}

func TestRenderReportMissingPredictionMarked(t *testing.T) {
	golds := []eval.GoldEmail{{ID: "1", GoldUrgency: "low", GoldType: "general"}}
	out := eval.Render(golds, map[string]eval.Prediction{}, 0.90) // no prediction for g1
	assert.Contains(t, out, "missing prediction")
}
