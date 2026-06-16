# Plan 6 — Eval Harness, Gold Dataset & Threshold Calibration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `make eval` harness that runs a hand-labeled gold dataset of support emails through the classifier and prints a quality report — overall accuracy, per-class precision/recall/F1, confusion matrices for urgency and type, and a calibration sweep that recommends the HITL confidence threshold — so the threshold is *calibrated, not guessed*.

**Architecture:** Pure, unit-tested metrics live in `internal/eval` (no I/O, no network). A thin `cmd/eval` binary is the only glue: it loads the gold set, gets predictions (replay from a committed cache, or live from Qwen, or from the deterministic fake classifier), and prints the report. `make eval` replays a committed cache (free, deterministic, no API key); `make eval-live` calls real Qwen once and rewrites the cache (spends quota only when asked). The committed cache is bootstrapped from the deterministic fake classifier so `make eval` works out of the box and is fully reproducible by anyone.

**Tech Stack:** Go 1.25; stdlib only (`encoding/json`, `text/tabwriter`, `flag`); reuses `internal/domain` enums, `internal/qwen.Client`, and `internal/classify.Fake`. Tests: `testify` with in-line fixtures.

**Design decisions (locked in brainstorming, 2026-06-12):**
- **Record-then-replay.** `make eval` reads `eval/recorded.json`; `make eval-live` records via Qwen. Quota spent only on refresh.
- **~30 hand-authored, balanced gold emails** spanning every urgency and type, including a few `critical` and a few deliberately ambiguous ones.
- **Calibration recommends only** — prints the suggested `CONFIDENCE_THRESHOLD`; a human copies it into `app.env`. The harness never writes config.
- **Evaluated dimensions:** urgency and type (department is derived from type via `domain.DepartmentForType`, so it follows). A prediction is "correct" when **both** urgency and type match the gold label.
- **Not a gate.** The harness is a report, never pass/fail; it does not run in CI against live Qwen.

---

## File structure

```
eval/gold.jsonl              → ~30 labeled examples, one JSON object per line (new)
eval/recorded.json           → cache: id → prediction; committed, refreshed by make eval-live (new)
internal/eval/data.go        → GoldEmail, Prediction types; LoadGold, LoadPredictions, SavePredictions (new)
internal/eval/data_test.go   → loader round-trip tests (new)
internal/eval/metrics.go     → accuracy, per-class P/R/F1, confusion matrix (new)
internal/eval/metrics_test.go→ metrics on a tiny known fixture (new)
internal/eval/calibrate.go   → threshold sweep + recommendation (new)
internal/eval/calibrate_test.go → calibration on a known fixture (new)
internal/eval/report.go      → text report renderer (new)
internal/eval/report_test.go → renderer smoke test (new)
cmd/eval/main.go             → glue: load gold, get predictions, print report (new)
Makefile                     → add `eval` + `eval-live` targets (modify)
CLAUDE.md                    → document the eval harness (modify)
```

All `internal/eval` functions are pure (slices in, structs out). `cmd/eval` is the only place that touches the filesystem, env, or network.

---

## Shared types (defined in Task 1, referenced everywhere)

```go
// GoldEmail is one labeled example from eval/gold.jsonl.
type GoldEmail struct {
	ID          string            `json:"id"`
	From        string            `json:"from"`
	Subject     string            `json:"subject"`
	Body        string            `json:"body"`
	GoldUrgency domain.Urgency    `json:"gold_urgency"`
	GoldType    domain.TicketType `json:"gold_type"`
}

// Prediction is a model output for one gold example (one entry in eval/recorded.json).
type Prediction struct {
	ID         string            `json:"id"`
	Urgency    domain.Urgency    `json:"urgency"`
	Type       domain.TicketType `json:"type"`
	Department domain.Department  `json:"department"`
	Confidence float64           `json:"confidence"`
}

// Correct reports whether the prediction matches the gold on BOTH dimensions.
func (p Prediction) Correct(g GoldEmail) bool {
	return p.Urgency == g.GoldUrgency && p.Type == g.GoldType
}
```

Ordered label sets used for confusion matrices and per-class rows:

```go
var UrgencyLabels = []string{"low", "normal", "high", "critical"}
var TypeLabels    = []string{"billing", "technical", "account", "feature_request", "general"}
```

---

## Task 1: Gold dataset + loader

**Files:**
- Create: `eval/gold.jsonl`
- Create: `internal/eval/data.go`
- Test: `internal/eval/data_test.go`

- [ ] **Step 1: Write the gold dataset** `eval/gold.jsonl` (one JSON object per line, no trailing comma, ~30 examples balanced across the taxonomy). Use exactly this content:

```jsonl
{"id":"g01","from":"alice@acme.com","subject":"Charged twice for my subscription","body":"I was billed $49 twice this month. Please refund the duplicate charge.","gold_urgency":"normal","gold_type":"billing"}
{"id":"g02","from":"bob@acme.com","subject":"Invoice does not match my plan","body":"My invoice shows the Pro price but I am on the Basic plan. Can you correct this?","gold_urgency":"normal","gold_type":"billing"}
{"id":"g03","from":"carol@acme.com","subject":"URGENT: double payment took my account negative","body":"You charged me twice and now my bank account is overdrawn. I need this refunded immediately.","gold_urgency":"high","gold_type":"billing"}
{"id":"g04","from":"dan@acme.com","subject":"Question about annual billing discount","body":"Do you offer a discount if I pay yearly instead of monthly?","gold_urgency":"low","gold_type":"billing"}
{"id":"g05","from":"erin@acme.com","subject":"Refund still not received","body":"You approved my refund two weeks ago but I still have not seen it. Please follow up.","gold_urgency":"normal","gold_type":"billing"}
{"id":"g06","from":"frank@acme.com","subject":"Production API returning 500 for all calls","body":"Every request to your API has returned HTTP 500 for the last 20 minutes. Our checkout is completely down.","gold_urgency":"critical","gold_type":"technical"}
{"id":"g07","from":"gina@acme.com","subject":"Dashboard charts not loading","body":"The analytics charts spin forever and never render in Chrome. Other pages work fine.","gold_urgency":"normal","gold_type":"technical"}
{"id":"g08","from":"hugo@acme.com","subject":"Intermittent timeouts on webhook delivery","body":"About 1 in 10 webhooks time out. It is not blocking us yet but it is getting worse.","gold_urgency":"high","gold_type":"technical"}
{"id":"g09","from":"ivy@acme.com","subject":"Export to CSV produces an empty file","body":"When I export my report to CSV the downloaded file has headers but no rows.","gold_urgency":"normal","gold_type":"technical"}
{"id":"g10","from":"jack@acme.com","subject":"Complete outage - site is down for everyone","body":"Your status page is green but nobody on our team can load the app at all. This is an emergency.","gold_urgency":"critical","gold_type":"technical"}
{"id":"g11","from":"kate@acme.com","subject":"Cannot log in - password reset email never arrives","body":"I requested a password reset three times and no email comes. I am locked out of my account.","gold_urgency":"high","gold_type":"account"}
{"id":"g12","from":"liam@acme.com","subject":"How do I change my account email address?","body":"I want to update the email on my account to a new work address. Where is that setting?","gold_urgency":"low","gold_type":"account"}
{"id":"g13","from":"mia@acme.com","subject":"Account locked after too many login attempts","body":"My account is locked. I need access restored so I can finish onboarding my team today.","gold_urgency":"high","gold_type":"account"}
{"id":"g14","from":"noah@acme.com","subject":"Add a teammate to my workspace","body":"How can I invite another user to my workspace and set their role to viewer?","gold_urgency":"low","gold_type":"account"}
{"id":"g15","from":"olivia@acme.com","subject":"Two-factor authentication is not working","body":"My authenticator codes are rejected every time. I cannot get into my account.","gold_urgency":"high","gold_type":"account"}
{"id":"g16","from":"pat@acme.com","subject":"Feature request: dark mode for the dashboard","body":"It would be great if the dashboard had a dark theme for late-night work.","gold_urgency":"low","gold_type":"feature_request"}
{"id":"g17","from":"quinn@acme.com","subject":"Please add Slack notifications","body":"A native Slack integration for alerts would save us building our own.","gold_urgency":"low","gold_type":"feature_request"}
{"id":"g18","from":"rosa@acme.com","subject":"Bulk edit would be a huge time saver","body":"Could you add the ability to edit multiple records at once? We do this daily.","gold_urgency":"normal","gold_type":"feature_request"}
{"id":"g19","from":"sam@acme.com","subject":"Suggestion: keyboard shortcuts","body":"Adding keyboard shortcuts for common actions would speed up our workflow a lot.","gold_urgency":"low","gold_type":"feature_request"}
{"id":"g20","from":"tina@acme.com","subject":"Wishlist: export to Google Sheets","body":"Direct export to Google Sheets instead of CSV would be a nice addition someday.","gold_urgency":"low","gold_type":"feature_request"}
{"id":"g21","from":"uma@acme.com","subject":"Where can I find your data processing agreement?","body":"Our legal team needs a copy of your DPA for our records. Where do I download it?","gold_urgency":"low","gold_type":"general"}
{"id":"g22","from":"vic@acme.com","subject":"Do you have a phone support line?","body":"I would prefer to talk to someone. Is there a number I can call?","gold_urgency":"low","gold_type":"general"}
{"id":"g23","from":"wes@acme.com","subject":"Thanks for the great product","body":"Just wanted to say the team loves the new release. Keep it up!","gold_urgency":"low","gold_type":"general"}
{"id":"g24","from":"xena@acme.com","subject":"What are your business hours?","body":"What timezone is your support team in and when are you available?","gold_urgency":"low","gold_type":"general"}
{"id":"g25","from":"yan@acme.com","subject":"Need help, not sure what is wrong","body":"Something seems off with my account and a charge looks weird. Can someone take a look?","gold_urgency":"normal","gold_type":"billing"}
{"id":"g26","from":"zoe@acme.com","subject":"It is broken","body":"Nothing works. Please help.","gold_urgency":"normal","gold_type":"technical"}
{"id":"g27","from":"amir@acme.com","subject":"URGENT data deletion request","body":"We are off-boarding a client and must delete their data today to meet a compliance deadline.","gold_urgency":"high","gold_type":"account"}
{"id":"g28","from":"bea@acme.com","subject":"Security: possible unauthorized access","body":"I see logins from a country we do not operate in. Please lock the account and investigate now.","gold_urgency":"critical","gold_type":"account"}
{"id":"g29","from":"cy@acme.com","subject":"Payment method declined repeatedly","body":"My card keeps getting declined at checkout even though it works elsewhere. I cannot renew.","gold_urgency":"high","gold_type":"billing"}
{"id":"g30","from":"deb@acme.com","subject":"General question about your roadmap","body":"Is there a public roadmap where I can see what features are coming next?","gold_urgency":"low","gold_type":"general"}
```

- [ ] **Step 2: Write the failing loader test** `internal/eval/data_test.go`:

```go
package eval_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lemonishi/autocierge/internal/eval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGold(t *testing.T) {
	// Loads the real committed dataset.
	golds, err := eval.LoadGold("../../eval/gold.jsonl")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(golds), 30)

	// Every label must be a valid enum value (catches typos in the dataset).
	for _, g := range golds {
		assert.NotEmpty(t, g.ID, "id must be set")
		assert.True(t, domainValidUrgency(g.GoldUrgency), "bad urgency %q in %s", g.GoldUrgency, g.ID)
		assert.True(t, domainValidType(g.GoldType), "bad type %q in %s", g.GoldType, g.ID)
	}

	// IDs are unique.
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

	// Saved file is valid JSON on disk.
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "g01")
}
```

Add these helpers at the bottom of the same test file (the test asserts dataset validity using the domain validators):

```go
import dom "github.com/lemonishi/autocierge/internal/domain"

func domainValidUrgency(u dom.Urgency) bool  { return dom.ValidUrgency(u) }
func domainValidType(t dom.TicketType) bool  { return dom.ValidType(t) }
```

(Place the `dom` import in the import block with the others; shown separately only for clarity.)

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/eval/ -run 'LoadGold|RoundTrip' -v`
Expected: FAIL — `package .../internal/eval` does not exist / undefined `eval.LoadGold`.

- [ ] **Step 4: Implement** `internal/eval/data.go`:

```go
// Package eval computes classification-quality metrics for the gold dataset:
// accuracy, per-class precision/recall/F1, confusion matrices, and a confidence
// threshold calibration sweep. All functions here are pure; cmd/eval is the only
// glue that performs I/O or calls the model.
package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/lemonishi/autocierge/internal/domain"
)

// GoldEmail is one labeled example from eval/gold.jsonl.
type GoldEmail struct {
	ID          string            `json:"id"`
	From        string            `json:"from"`
	Subject     string            `json:"subject"`
	Body        string            `json:"body"`
	GoldUrgency domain.Urgency    `json:"gold_urgency"`
	GoldType    domain.TicketType `json:"gold_type"`
}

// Prediction is a model output for one gold example (one entry in eval/recorded.json).
type Prediction struct {
	ID         string            `json:"id"`
	Urgency    domain.Urgency    `json:"urgency"`
	Type       domain.TicketType `json:"type"`
	Department domain.Department `json:"department"`
	Confidence float64           `json:"confidence"`
}

// Correct reports whether the prediction matches the gold on BOTH dimensions.
func (p Prediction) Correct(g GoldEmail) bool {
	return p.Urgency == g.GoldUrgency && p.Type == g.GoldType
}

// Ordered label sets for per-class rows and confusion matrices.
var (
	UrgencyLabels = []string{"low", "normal", "high", "critical"}
	TypeLabels    = []string{"billing", "technical", "account", "feature_request", "general"}
)

// LoadGold reads a JSONL gold dataset (one GoldEmail per line).
func LoadGold(path string) ([]GoldEmail, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("eval: open gold: %w", err)
	}
	defer f.Close()

	var out []GoldEmail
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var g GoldEmail
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("eval: gold line %d: %w", line, err)
		}
		out = append(out, g)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("eval: scan gold: %w", err)
	}
	return out, nil
}

// LoadPredictions reads the recorded-prediction cache (id -> Prediction).
func LoadPredictions(path string) (map[string]Prediction, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eval: read predictions: %w", err)
	}
	var m map[string]Prediction
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("eval: parse predictions: %w", err)
	}
	return m, nil
}

// SavePredictions writes the prediction cache as indented JSON (stable key order,
// so git diffs are clean). The file is created with 0o644.
func SavePredictions(path string, m map[string]Prediction) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("eval: marshal predictions: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("eval: write predictions: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/eval/ -run 'LoadGold|RoundTrip' -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add eval/gold.jsonl internal/eval/data.go internal/eval/data_test.go
git commit -m "feat(eval): gold dataset + loader and prediction cache I/O"
```

---

## Task 2: Metrics — accuracy, per-class P/R/F1, confusion matrix

**Files:**
- Create: `internal/eval/metrics.go`
- Test: `internal/eval/metrics_test.go`

- [ ] **Step 1: Write the failing test** `internal/eval/metrics_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/eval/ -run Dimension -v`
Expected: FAIL — undefined `eval.Dimension` / `eval.ClassMetrics`.

- [ ] **Step 3: Implement** `internal/eval/metrics.go`:

```go
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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/eval/ -run Dimension -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/eval/metrics.go internal/eval/metrics_test.go
git commit -m "feat(eval): accuracy, per-class P/R/F1, and confusion matrix"
```

---

## Task 3: Threshold calibration sweep

**Files:**
- Create: `internal/eval/calibrate.go`
- Test: `internal/eval/calibrate_test.go`

The sweep treats `confidence >= threshold` as "auto-route" and measures how accurate those confident predictions are. It recommends the **lowest threshold whose auto-route accuracy meets the target** (default 0.90), so the system auto-routes as much as it safely can. If no threshold meets the target, it recommends the highest swept threshold and flags `TargetMet = false`.

- [ ] **Step 1: Write the failing test** `internal/eval/calibrate_test.go`:

```go
package eval_test

import (
	"testing"

	"github.com/lemonishi/autocierge/internal/eval"
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
		// Two confident + correct, one confident + WRONG (low conf), one mid + correct.
		"1": {ID: "1", Urgency: "normal", Type: "billing", Confidence: 0.95}, // correct
		"2": {ID: "2", Urgency: "normal", Type: "billing", Confidence: 0.90}, // correct
		"3": {ID: "3", Urgency: "high", Type: "technical", Confidence: 0.60}, // WRONG, low conf
		"4": {ID: "4", Urgency: "normal", Type: "billing", Confidence: 0.80}, // correct
	}

	cal := eval.Calibrate(golds, preds, 0.90)

	// At threshold 0.80: auto-routes ids 1,2,4 (all correct) -> accuracy 1.0 >= 0.90.
	// At threshold below that the wrong low-conf one is excluded anyway (0.60),
	// so 0.65 would also be 100% accurate over {1,2,4}. The lowest swept threshold
	// that auto-routes only correct ones and meets target should be recommended.
	assert.True(t, cal.TargetMet)
	assert.LessOrEqual(t, cal.Recommended, 0.80)
	assert.InDelta(t, 0.90, cal.TargetAccuracy, 1e-9)

	// Rows cover the swept range and carry coverage + accuracy.
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
		// Both wrong but highly confident -> no threshold reaches 0.90 accuracy.
		"1": {ID: "1", Urgency: "high", Type: "technical", Confidence: 0.99},
		"2": {ID: "2", Urgency: "high", Type: "technical", Confidence: 0.99},
	}
	cal := eval.Calibrate(golds, preds, 0.90)
	assert.False(t, cal.TargetMet)
	assert.InDelta(t, 0.95, cal.Recommended, 1e-9) // most conservative swept threshold
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/eval/ -run Calibrate -v`
Expected: FAIL — undefined `eval.Calibrate`.

- [ ] **Step 3: Implement** `internal/eval/calibrate.go`:

```go
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
	// Build sweep low->high so the first qualifying row is the lowest threshold.
	for t := lo; t <= hi+1e-9; t += step {
		thr := round2(t)
		var routed, correct int
		for _, g := range golds {
			p, ok := preds[g.ID]
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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/eval/ -run Calibrate -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/eval/calibrate.go internal/eval/calibrate_test.go
git commit -m "feat(eval): confidence threshold calibration sweep"
```

---

## Task 4: Text report renderer

**Files:**
- Create: `internal/eval/report.go`
- Test: `internal/eval/report_test.go`

- [ ] **Step 1: Write the failing test** `internal/eval/report_test.go`:

```go
package eval_test

import (
	"strings"
	"testing"

	"github.com/lemonishi/autocierge/internal/eval"
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

	// Headline sections are present.
	assert.Contains(t, out, "URGENCY")
	assert.Contains(t, out, "TYPE")
	assert.Contains(t, out, "Confusion")
	assert.Contains(t, out, "Calibration")
	assert.Contains(t, out, "Recommended CONFIDENCE_THRESHOLD")
	// Overall accuracy line.
	assert.Contains(t, out, "Overall accuracy")
	// It should not panic on a fully-correct set and should report 100%.
	assert.True(t, strings.Contains(out, "100.0%") || strings.Contains(out, "1.000"))
}

func TestRenderReportMissingPredictionMarked(t *testing.T) {
	golds := []eval.GoldEmail{{ID: "1", GoldUrgency: "low", GoldType: "general"}}
	out := eval.Render(golds, map[string]eval.Prediction{}, 0.90) // no prediction for g1
	assert.Contains(t, out, "missing prediction")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/eval/ -run Render -v`
Expected: FAIL — undefined `eval.Render`.

- [ ] **Step 3: Implement** `internal/eval/report.go`:

```go
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

	// Align gold/pred slices, tracking any missing predictions.
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

	// Confusion matrix (rows = gold, cols = predicted).
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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/eval/ -run Render -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/eval/report.go internal/eval/report_test.go
git commit -m "feat(eval): plain-text quality report renderer"
```

---

## Task 5: `cmd/eval` glue, Makefile targets, and committed cache

**Files:**
- Create: `cmd/eval/main.go`
- Modify: `Makefile` (add `eval`, `eval-live`; extend `.PHONY`)
- Create: `eval/recorded.json` (bootstrapped from the fake classifier in Step 5)

`cmd/eval` has three modes:
- **default (replay):** read `eval/recorded.json`, render the report. No network, no API key.
- **`--live`:** classify every gold email with real Qwen, rewrite the cache, then render. Needs `DASHSCOPE_API_KEY`.
- **`--fake`:** classify with the deterministic `classify.Fake`, rewrite the cache, then render. Used to bootstrap/regenerate the committed cache reproducibly without quota; also handy in CI.

To avoid requiring `DATABASE_URL` (the eval is DB-free), `cmd/eval` reads DashScope settings directly from the environment with the same defaults as `internal/config`.

- [ ] **Step 1: Implement** `cmd/eval/main.go`:

```go
// Command eval runs the gold dataset through the classifier and prints a quality
// report: accuracy, per-class precision/recall/F1, confusion matrices, and a
// confidence-threshold calibration sweep.
//
//	go run ./cmd/eval            # replay the committed cache (eval/recorded.json)
//	go run ./cmd/eval --live     # classify with real Qwen, refresh the cache
//	go run ./cmd/eval --fake     # classify with the deterministic fake, refresh the cache
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/lemonishi/autocierge/internal/classify"
	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/lemonishi/autocierge/internal/eval"
	"github.com/lemonishi/autocierge/internal/qwen"
)

func main() {
	var (
		live      = flag.Bool("live", false, "classify with real Qwen and refresh the cache")
		fake      = flag.Bool("fake", false, "classify with the deterministic fake classifier and refresh the cache")
		goldPath  = flag.String("gold", "eval/gold.jsonl", "path to the gold dataset (JSONL)")
		cachePath = flag.String("cache", "eval/recorded.json", "path to the prediction cache (JSON)")
		target    = flag.Float64("target", 0.90, "target auto-route accuracy for threshold calibration")
	)
	flag.Parse()

	golds, err := eval.LoadGold(*goldPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(golds) == 0 {
		log.Fatalf("eval: gold dataset %s is empty", *goldPath)
	}

	var preds map[string]eval.Prediction
	switch {
	case *live && *fake:
		log.Fatal("eval: choose --live or --fake, not both")
	case *live:
		clf := qwen.New(
			os.Getenv("DASHSCOPE_API_KEY"),
			getenv("DASHSCOPE_BASE_URL", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"),
			getenv("QWEN_MODEL", "qwen-max"),
			nil,
		)
		if os.Getenv("DASHSCOPE_API_KEY") == "" {
			log.Fatal("eval: --live requires DASHSCOPE_API_KEY")
		}
		preds = record(golds, clf)
		if err := eval.SavePredictions(*cachePath, preds); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("recorded %d live predictions to %s\n\n", len(preds), *cachePath)
	case *fake:
		preds = record(golds, classify.NewFake())
		if err := eval.SavePredictions(*cachePath, preds); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("recorded %d fake-classifier predictions to %s\n\n", len(preds), *cachePath)
	default:
		preds, err = eval.LoadPredictions(*cachePath)
		if err != nil {
			log.Fatalf("%v\n(run `make eval-live` or `go run ./cmd/eval --fake` to create the cache)", err)
		}
	}

	fmt.Print(eval.Render(golds, preds, *target))
}

// record classifies every gold email and returns id -> Prediction.
func record(golds []eval.GoldEmail, clf domain.Classifier) map[string]eval.Prediction {
	out := make(map[string]eval.Prediction, len(golds))
	for _, g := range golds {
		email := domain.Email{FromAddr: g.From, Subject: g.Subject, Body: g.Body}
		c, err := clf.Classify(context.Background(), email)
		if err != nil {
			log.Fatalf("eval: classify %s: %v", g.ID, err)
		}
		out[g.ID] = eval.Prediction{
			ID: g.ID, Urgency: c.Urgency, Type: c.Type,
			Department: c.Department, Confidence: c.Confidence,
		}
	}
	return out
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./cmd/eval`
Expected: builds with no output.

- [ ] **Step 3: Bootstrap the committed cache from the fake classifier**

Run: `go run ./cmd/eval --fake`
Expected: prints `recorded 30 fake-classifier predictions to eval/recorded.json` followed by the full report (accuracy, per-class tables, confusion matrices, calibration sweep, recommended threshold). Confirms the whole pipeline works end-to-end and creates `eval/recorded.json`.

- [ ] **Step 4: Verify replay mode reads the committed cache**

Run: `go run ./cmd/eval`
Expected: prints the same report (no "recorded ..." line), proving replay works with no API key and no network.

- [ ] **Step 5: Add Makefile targets.** Modify the `.PHONY` line to include `eval eval-live`, and append these targets at the end of `Makefile`:

```make
# Classification quality report. `make eval` replays the committed cache
# (eval/recorded.json) — free, deterministic, no API key. `make eval-live`
# calls real Qwen on the gold set and refreshes the cache (spends quota).
eval:
	go run ./cmd/eval

eval-live:
	go run ./cmd/eval --live
```

Update the first line of the Makefile from:

```make
.PHONY: dev run test test-db build tidy frontend
```
to:
```make
.PHONY: dev run test test-db build tidy frontend eval eval-live
```

- [ ] **Step 6: Verify the Make targets**

Run: `make eval`
Expected: the report prints (replay mode).

- [ ] **Step 7: Commit**

```bash
git add cmd/eval/main.go eval/recorded.json Makefile
git commit -m "feat(eval): cmd/eval (replay/live/fake) + make eval targets + bootstrapped cache"
```

---

## Task 6: Documentation + full verification

**Files:**
- Modify: `CLAUDE.md` (add an Evaluation section to the Stack list)

- [ ] **Step 1: Document the harness in** `CLAUDE.md`. Add this bullet to the Stack list, immediately after the `Alerting:` bullet:

```markdown
- Evaluation: `internal/eval` + `cmd/eval` — gold dataset (`eval/gold.jsonl`, ~30
  labeled support emails) run through the classifier to produce a quality report:
  overall accuracy, per-class precision/recall/F1, confusion matrices (urgency &
  type), and a confidence-threshold calibration sweep that recommends
  `CONFIDENCE_THRESHOLD` (calibrated, not guessed). `make eval` replays a committed
  cache (`eval/recorded.json`, bootstrapped from the fake classifier — free,
  deterministic, no API key); `make eval-live` refreshes it via real Qwen (spends
  quota). The report recommends the threshold; a human sets it in `app.env`. Pure
  metrics in `internal/eval` are unit-tested; the report is the source of demo metrics.
```

- [ ] **Step 2: Run the full eval package test suite**

Run: `go test ./internal/eval/ -v`
Expected: all tests PASS (data, metrics, calibration, report).

- [ ] **Step 3: Run vet, build, and the whole suite**

Run: `go vet ./... && go build ./... && go test ./...`
Expected: vet clean; build clean; all packages green (including the existing 12 plus `internal/eval`).

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document the eval harness in CLAUDE.md"
```

---

## Verification checklist

- [ ] `make eval` prints a full report with no API key and no network (replay mode).
- [ ] `make eval-live` refreshes `eval/recorded.json` via real Qwen (manual; needs `DASHSCOPE_API_KEY` and available quota).
- [ ] `eval/gold.jsonl` has ~30 examples spanning every urgency and type, all labels valid enums, unique ids.
- [ ] The report shows overall accuracy, per-class P/R/F1 for urgency and type, both confusion matrices, the calibration sweep, and a recommended `CONFIDENCE_THRESHOLD`.
- [ ] Calibration recommends the lowest threshold meeting the target accuracy (or flags TARGET NOT MET).
- [ ] A missing prediction in the cache surfaces as a WARNING, not a silent skip.
- [ ] `internal/eval` is pure and unit-tested; `cmd/eval` is the only I/O/network glue.
- [ ] `go vet ./...`, `go build ./...`, `go test ./...` all clean/green.
- [ ] The harness never writes to `app.env` or any config — it only recommends.

## Manual follow-up (user, when quota is available)

- [ ] Run `make eval-live` to record real Qwen predictions, then review the recommended `CONFIDENCE_THRESHOLD` and set it in `app.env`. This is the demo-metrics artifact for the submission.
