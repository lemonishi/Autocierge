package eval_test

import (
	"testing"

	"github.com/lemonishi/supportsentinel/internal/eval"
	"github.com/stretchr/testify/assert"
)

func TestCalibrateRecommendsLowestThresholdMeetingTarget(t *testing.T) {
	golds := []eval.GoldEmail{
		{ID: "1", GoldUrgency: "normal", GoldType: "billing"},
		{ID: "2", GoldUrgency: "normal", GoldType: "billing"},
		{ID: "3", GoldUrgency: "normal", GoldType: "billing"},
		{ID: "4", GoldUrgency: "normal", GoldType: "billing"},
	}
	preds := map[string]eval.Prediction{
		"1": {ID: "1", Urgency: "normal", Type: "billing", Confidence: 0.95}, // correct
		"2": {ID: "2", Urgency: "normal", Type: "billing", Confidence: 0.90}, // correct
		"3": {ID: "3", Urgency: "high", Type: "technical", Confidence: 0.60}, // WRONG, low conf
		"4": {ID: "4", Urgency: "normal", Type: "billing", Confidence: 0.80}, // correct
	}

	cal := eval.Calibrate(golds, preds, 0.90)

	assert.True(t, cal.TargetMet)
	assert.LessOrEqual(t, cal.Recommended, 0.80)
	assert.InDelta(t, 0.90, cal.TargetAccuracy, 1e-9)

	assert.NotEmpty(t, cal.Rows)
	for _, r := range cal.Rows {
		assert.GreaterOrEqual(t, r.Threshold, 0.50)
		assert.LessOrEqual(t, r.Threshold, 0.95)
	}
}

func TestCalibrateTargetUnreachable(t *testing.T) {
	golds := []eval.GoldEmail{
		{ID: "1", GoldUrgency: "normal", GoldType: "billing"},
		{ID: "2", GoldUrgency: "normal", GoldType: "billing"},
	}
	preds := map[string]eval.Prediction{
		"1": {ID: "1", Urgency: "high", Type: "technical", Confidence: 0.99},
		"2": {ID: "2", Urgency: "high", Type: "technical", Confidence: 0.99},
	}
	cal := eval.Calibrate(golds, preds, 0.90)
	assert.False(t, cal.TargetMet)
	assert.InDelta(t, 0.95, cal.Recommended, 1e-9)
}
