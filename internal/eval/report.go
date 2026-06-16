package eval

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

// Render builds the full plain-text quality report for the gold set against the
// recorded predictions. Gold examples with no prediction are counted as wrong
// (predicted ""), and a "missing prediction" note is emitted listing their ids —
// so a stale cache is visible rather than silently skipping examples.
func Render(golds []GoldEmail, preds map[string]Prediction, target float64) string {
	var b strings.Builder

	var urgGold, urgPred, typGold, typPred []string
	var missing []string
	overallCorrect := 0
	for _, g := range golds {
		p, ok := preds[g.ID]
		if !ok {
			missing = append(missing, g.ID)
		}
		urgGold = append(urgGold, string(g.GoldUrgency))
		urgPred = append(urgPred, string(p.Urgency)) // "" when missing
		typGold = append(typGold, string(g.GoldType))
		typPred = append(typPred, string(p.Type))
		if ok && p.Correct(g) {
			overallCorrect++
		}
	}

	fmt.Fprintf(&b, "Autocierge — Classification Quality Report\n")
	fmt.Fprintf(&b, "Gold examples: %d\n", len(golds))
	overall := 0.0
	if len(golds) > 0 {
		overall = float64(overallCorrect) / float64(len(golds))
	}
	fmt.Fprintf(&b, "Overall accuracy (urgency AND type both correct): %.1f%%\n", overall*100)
	if len(missing) > 0 {
		fmt.Fprintf(&b, "WARNING: missing prediction for %d example(s): %s\n",
			len(missing), strings.Join(missing, ", "))
		fmt.Fprintf(&b, "  Run `make eval-live` (or regenerate the cache) to refresh predictions.\n")
	}
	b.WriteString("\n")

	urg := Dimension("urgency", UrgencyLabels, urgGold, urgPred)
	typ := Dimension("type", TypeLabels, typGold, typPred)
	writeDimension(&b, urg)
	writeDimension(&b, typ)

	cal := Calibrate(golds, preds, target)
	writeCalibration(&b, cal)

	return b.String()
}

func writeDimension(b *strings.Builder, d DimensionReport) {
	fmt.Fprintf(b, "== %s ==\n", strings.ToUpper(d.Name))
	fmt.Fprintf(b, "Accuracy: %.1f%%\n", d.Accuracy*100)

	tw := tabwriter.NewWriter(b, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "class\tsupport\tprecision\trecall\tF1")
	for _, c := range d.PerClass {
		fmt.Fprintf(tw, "%s\t%d\t%.2f\t%.2f\t%.2f\n", c.Label, c.Support, c.Precision, c.Recall, c.F1)
	}
	tw.Flush()

	fmt.Fprintf(b, "Confusion (rows=gold, cols=pred):\n")
	ctw := tabwriter.NewWriter(b, 0, 2, 2, ' ', 0)
	fmt.Fprintf(ctw, "gold\\pred\t%s\n", strings.Join(d.Labels, "\t"))
	for _, g := range d.Labels {
		cells := make([]string, len(d.Labels))
		for i, p := range d.Labels {
			cells[i] = fmt.Sprintf("%d", d.Confusion[g][p])
		}
		fmt.Fprintf(ctw, "%s\t%s\n", g, strings.Join(cells, "\t"))
	}
	ctw.Flush()
	b.WriteString("\n")
}

func writeCalibration(b *strings.Builder, cal Calibration) {
	fmt.Fprintf(b, "== Calibration (auto-route when confidence >= threshold) ==\n")
	tw := tabwriter.NewWriter(b, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "threshold\tauto-routed\tauto-route acc\tcoverage")
	for _, r := range cal.Rows {
		fmt.Fprintf(tw, "%.2f\t%d\t%.1f%%\t%.1f%%\n",
			r.Threshold, r.AutoRouted, r.AutoRouteAcc*100, r.Coverage*100)
	}
	tw.Flush()

	fmt.Fprintf(b, "\nTarget auto-route accuracy: %.0f%%\n", cal.TargetAccuracy*100)
	if cal.TargetMet {
		fmt.Fprintf(b, "Recommended CONFIDENCE_THRESHOLD: %.2f\n", cal.Recommended)
		fmt.Fprintf(b, "  (lowest threshold meeting the target — routes the most while keeping misroutes rare)\n")
	} else {
		fmt.Fprintf(b, "Recommended CONFIDENCE_THRESHOLD: %.2f  (TARGET NOT MET — most conservative swept value)\n", cal.Recommended)
		fmt.Fprintf(b, "  No threshold reached the target accuracy; consider improving the prompt or dataset.\n")
	}
	fmt.Fprintf(b, "Set it in app.env: CONFIDENCE_THRESHOLD=%.2f\n", cal.Recommended)
}
