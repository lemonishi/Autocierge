package eval

// ThresholdRow is one row of the calibration sweep.
type ThresholdRow struct {
	Threshold    float64
	AutoRouted   int     // predictions with confidence >= threshold
	Correct      int     // of those, how many were correct on both dimensions
	AutoRouteAcc float64 // Correct / AutoRouted (0 when AutoRouted == 0)
	Coverage     float64 // AutoRouted / total
}

// Calibration is the full sweep plus the recommended threshold.
type Calibration struct {
	Rows           []ThresholdRow
	Recommended    float64
	TargetAccuracy float64
	TargetMet      bool
}

// Calibrate sweeps thresholds from 0.50 to 0.95 (step 0.05). For each it measures
// auto-route accuracy (correctness among predictions at/above the threshold) and
// coverage. It recommends the LOWEST threshold whose auto-route accuracy meets
// target while auto-routing at least one prediction. If none qualifies, it
// recommends the highest swept threshold and sets TargetMet=false.
func Calibrate(golds []GoldEmail, preds map[string]Prediction, target float64) Calibration {
	total := len(golds)
	cal := Calibration{TargetAccuracy: target, Recommended: 0.95}

	const lo, hi, step = 0.50, 0.95, 0.05
	for t := lo; t <= hi+1e-9; t += step {
		thr := round2(t)
		var routed, correct int
		for _, g := range golds {
			p, ok := preds[g.ID]
			// A gold with no prediction is never auto-routed (it would park for
			// review), so it is excluded from the sweep rather than counted wrong
			// the way Render's overall/per-dimension accuracy treats it. With the
			// committed cache fully populated this branch never fires; it only
			// matters for a stale cache, which Render already flags as a WARNING.
			if !ok || p.Confidence < thr {
				continue
			}
			routed++
			if p.Correct(g) {
				correct++
			}
		}
		acc := 0.0
		if routed > 0 {
			acc = float64(correct) / float64(routed)
		}
		cov := 0.0
		if total > 0 {
			cov = float64(routed) / float64(total)
		}
		cal.Rows = append(cal.Rows, ThresholdRow{
			Threshold: thr, AutoRouted: routed, Correct: correct,
			AutoRouteAcc: acc, Coverage: cov,
		})
	}

	for _, r := range cal.Rows {
		if r.AutoRouted > 0 && r.AutoRouteAcc >= target {
			cal.Recommended = r.Threshold
			cal.TargetMet = true
			break
		}
	}
	return cal
}

// round2 rounds to 2 decimals to keep threshold values clean (0.50, 0.55, ...).
func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
