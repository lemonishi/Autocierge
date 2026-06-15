package eval

// ClassMetrics holds precision/recall/F1 for one class label.
type ClassMetrics struct {
	Label     string
	Support   int // number of gold examples with this label
	Precision float64
	Recall    float64
	F1        float64
}

// DimensionReport is the metrics for one classified dimension (urgency or type).
type DimensionReport struct {
	Name      string
	Accuracy  float64
	Labels    []string                  // ordered label set (matches the matrix axes)
	PerClass  []ClassMetrics            // one per label, in Labels order
	Confusion map[string]map[string]int // gold -> predicted -> count
}

// Dimension computes accuracy, per-class precision/recall/F1, and the confusion
// matrix for parallel gold/pred label slices. golds and preds must be the same
// length. labels is the ordered, complete label set for the dimension.
func Dimension(name string, labels []string, golds, preds []string) DimensionReport {
	conf := make(map[string]map[string]int, len(labels))
	for _, g := range labels {
		conf[g] = make(map[string]int, len(labels))
	}

	correct := 0
	for i := range golds {
		g, p := golds[i], preds[i]
		if conf[g] == nil {
			conf[g] = map[string]int{}
		}
		conf[g][p]++
		if g == p {
			correct++
		}
	}

	acc := 0.0
	if len(golds) > 0 {
		acc = float64(correct) / float64(len(golds))
	}

	per := make([]ClassMetrics, 0, len(labels))
	for _, c := range labels {
		var tp, fp, fn, support int
		for i := range golds {
			switch {
			case golds[i] == c && preds[i] == c:
				tp++
				support++
			case golds[i] != c && preds[i] == c:
				fp++
			case golds[i] == c && preds[i] != c:
				fn++
				support++
			}
		}
		prec := ratio(tp, tp+fp)
		rec := ratio(tp, tp+fn)
		f1 := 0.0
		if prec+rec > 0 {
			f1 = 2 * prec * rec / (prec + rec)
		}
		per = append(per, ClassMetrics{Label: c, Support: support, Precision: prec, Recall: rec, F1: f1})
	}

	return DimensionReport{Name: name, Accuracy: acc, Labels: labels, PerClass: per, Confusion: conf}
}

func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}
