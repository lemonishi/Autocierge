package eval_test

import (
	"testing"

	"github.com/lemonishi/autocierge/internal/eval"
	"github.com/stretchr/testify/assert"
)

func TestDimensionMetrics(t *testing.T) {
	labels := []string{"a", "b", "c"}
	golds := []string{"a", "a", "b", "b", "c"}
	preds := []string{"a", "b", "b", "b", "c"} // one "a" mispredicted as "b"

	d := eval.Dimension("demo", labels, golds, preds)

	// 4 of 5 correct.
	assert.InDelta(t, 0.8, d.Accuracy, 1e-9)

	byLabel := map[string]eval.ClassMetrics{}
	for _, c := range d.PerClass {
		byLabel[c.Label] = c
	}

	// Class "a": support 2, TP 1, FP 0, FN 1 -> precision 1.0, recall 0.5, F1 0.667.
	assert.Equal(t, 2, byLabel["a"].Support)
	assert.InDelta(t, 1.0, byLabel["a"].Precision, 1e-9)
	assert.InDelta(t, 0.5, byLabel["a"].Recall, 1e-9)
	assert.InDelta(t, 2.0/3.0, byLabel["a"].F1, 1e-9)

	// Class "b": support 2, TP 2, FP 1 (the mispredicted a), FN 0 -> precision 0.667, recall 1.0.
	assert.Equal(t, 2, byLabel["b"].Support)
	assert.InDelta(t, 2.0/3.0, byLabel["b"].Precision, 1e-9)
	assert.InDelta(t, 1.0, byLabel["b"].Recall, 1e-9)

	// Confusion: gold "a" -> {a:1, b:1}.
	assert.Equal(t, 1, d.Confusion["a"]["a"])
	assert.Equal(t, 1, d.Confusion["a"]["b"])
	assert.Equal(t, 2, d.Confusion["b"]["b"])
}

func TestDimensionZeroDivisionSafe(t *testing.T) {
	labels := []string{"a", "b"}
	// No predictions of "b" at all -> precision denom 0 must yield 0, not NaN.
	d := eval.Dimension("demo", labels, []string{"a", "a"}, []string{"a", "a"})
	byLabel := map[string]eval.ClassMetrics{}
	for _, c := range d.PerClass {
		byLabel[c.Label] = c
	}
	assert.Equal(t, 0.0, byLabel["b"].Precision)
	assert.Equal(t, 0.0, byLabel["b"].Recall)
	assert.Equal(t, 0.0, byLabel["b"].F1)
	assert.Equal(t, 1.0, d.Accuracy)
}
