# SupportSentinel — Plan 6: Evaluation Harness + Gold Dataset + Threshold Calibration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Read `HANDOVER.md` first if new.

**Goal:** Prove the classifier is actually good — a re-runnable harness over a labeled **gold dataset** that reports **accuracy, per-class precision/recall/F1, a confusion matrix, and confidence calibration**, and recommends a principled `CONFIDENCE_THRESHOLD` for the human-in-the-loop gate. This turns "the agent classifies tickets" from a claim into evidence (a number for the demo/writeup) and makes the Checkpoint-1 threshold defensible instead of guessed.

**Architecture:** A gold dataset (`eval/gold.json`) of labeled emails. Pure metric functions (`eval/metrics.go`) compute accuracy / per-class P/R/F1 / confusion matrix / confidence-bucket calibration from `(predicted, expected, confidence)` triples — fully unit-tested with synthetic data. A runner (`cmd/eval`) loads the gold set, runs each email through the real `domain.Classifier` (Qwen when `DASHSCOPE_API_KEY` is set, else the fake classifier), and prints the report; `make eval` invokes it. The harness reuses the existing classifier with no production changes.

**Tech Stack:** Go 1.25, stdlib only for metrics. Live runs use the real Qwen classifier (manual). Pure metrics are TDD'd.

**Spec:** `docs/superpowers/specs/2026-06-05-supportsentinel-design.md` (§3 component 8 — evaluation, §8 testing). **Builds on:** Plans 1–3 (`domain.Classifier`, `internal/qwen`, `internal/classify`). **Module path:** `github.com/lemonishi/supportsentinel`. Commit as `Lennon <lemoncode8888@gmail.com>` (see HANDOVER.md).

---

## File Structure (Plan 6)

```
eval/gold.json            → labeled gold dataset (new)
eval/dataset.go            → GoldCase type + Load() (new)
eval/dataset_test.go        → loader + dataset-integrity tests (new)
eval/metrics.go            → accuracy / per-class P-R-F1 / confusion matrix / calibration (pure) (new)
eval/metrics_test.go        → synthetic-data tests (new)
eval/report.go             → human-readable report formatting (new)
cmd/eval/main.go           → load gold → run classifier → print report + threshold rec (new)
Makefile                   → `eval` target (modify)
CLAUDE.md                  → note the eval harness (modify)
```

---

## Task 1: Gold dataset + loader

**Files:** create `eval/gold.json`, `eval/dataset.go`, `eval/dataset_test.go`.

- [ ] **Dataset** (`eval/gold.json`): start with **~30 cases** (note in the file that it should grow toward 50–100). Each case:
```json
{ "id": "g001",
  "from": "billing@acme.com",
  "subject": "I was charged twice this month",
  "body": "My card shows two identical charges for the annual plan.",
  "expected_urgency": "high",
  "expected_type": "billing" }
```
Deliberately include **ambiguous and adversarial** cases (vague complaints, angry-but-not-urgent, mixed signals, outages, feature requests phrased as complaints) so the eval reflects reality, not just easy wins. Cover all 5 types and all 4 urgency levels. (Department is derived from type, so it isn't labeled.)

- [ ] **Loader** (`eval/dataset.go`): `GoldCase` struct mirroring the JSON; `Load(path string) ([]GoldCase, error)` (use `//go:embed gold.json` for a default `LoadEmbedded()` so `cmd/eval` needs no path).

- [ ] **Test:** `LoadEmbedded()` returns ≥30 cases; every case has a valid `expected_urgency` and `expected_type` (use `domain.ValidUrgency`/`ValidType`); ids are unique; every type and urgency appears at least once (coverage guard). TDD.

- [ ] Commit: `feat(eval): gold dataset and loader`.

---

## Task 2: Metrics (pure)

**Files:** create `eval/metrics.go`, `eval/metrics_test.go`.

- [ ] **Test** with synthetic results (no classifier, no network): construct a slice of `Result{ExpectedType, PredictedType, ExpectedUrgency, PredictedUrgency, Confidence, Correct}` and assert:
  - overall **type accuracy** and **urgency accuracy** computed correctly on a hand-checked tiny set;
  - **per-class precision/recall/F1** for a known confusion (e.g. 2 billing correct, 1 billing predicted as technical) match hand-computed values;
  - **confusion matrix** counts are correct;
  - edge cases: a class with zero predictions → precision defined as 0 (not NaN); zero support → recall 0.
  TDD.

- [ ] **Implement** `eval/metrics.go` (pure functions):
```go
type Result struct {
	ID                string
	ExpectedType      domain.TicketType
	PredictedType     domain.TicketType
	ExpectedUrgency   domain.Urgency
	PredictedUrgency  domain.Urgency
	Confidence        float64
}
type ClassMetrics struct{ Precision, Recall, F1 float64; Support int }

func TypeAccuracy(rs []Result) float64
func UrgencyAccuracy(rs []Result) float64
func PerTypeMetrics(rs []Result) map[domain.TicketType]ClassMetrics
func ConfusionMatrix(rs []Result) map[domain.TicketType]map[domain.TicketType]int // expected → predicted → count
```
Guard divide-by-zero (precision/recall = 0 when denominator is 0). Per-class F1 = harmonic mean of P and R (0 if both 0).

- [ ] Commit: `feat(eval): accuracy, per-class P/R/F1, confusion matrix`.

---

## Task 3: Confidence calibration + threshold recommendation

**Files:** modify `eval/metrics.go`, `eval/metrics_test.go`.

- [ ] **Test:** given synthetic results where high-confidence predictions are mostly correct and low-confidence mostly wrong, `Calibration` buckets them (e.g. 0–0.5, 0.5–0.7, 0.7–0.85, 0.85–1.0) with per-bucket accuracy + count; `RecommendThreshold` returns the lowest confidence at/above which type-accuracy meets a target (e.g. ≥0.9) — i.e. the point where auto-routing is safe and everything below escalates to a human. TDD.

- [ ] **Implement:**
```go
type Bucket struct{ Low, High, Accuracy float64; Count int }
func Calibration(rs []Result) []Bucket
// RecommendThreshold returns the smallest confidence c such that predictions with
// confidence >= c have type-accuracy >= target; callers set CONFIDENCE_THRESHOLD to it.
func RecommendThreshold(rs []Result, targetAccuracy float64) float64
```

- [ ] Commit: `feat(eval): confidence calibration and threshold recommendation`.

---

## Task 4: Report + runner + `make eval`

**Files:** create `eval/report.go`, `cmd/eval/main.go`; modify `Makefile`, `CLAUDE.md`.

- [ ] **`eval/report.go`:** `Format(rs []Result) string` producing a readable report — overall type/urgency accuracy, a per-type table (P/R/F1/support), the confusion matrix as a grid, the calibration buckets, and the recommended threshold. (Light test: it contains the headline accuracy and "Recommended threshold".)

- [ ] **`cmd/eval/main.go`:**
```go
// Loads the embedded gold set, builds the classifier (Qwen if DASHSCOPE_API_KEY set,
// else fake), classifies every case, collects Results (Predicted* from the
// classification, Confidence from it), prints eval.Format(results) and the
// recommended threshold. Exits non-zero only on harness errors, not on low scores.
func main() {
	cfg, _ := config.Load() // DATABASE_URL not needed; tolerate its absence here OR set a dummy
	var clf domain.Classifier
	if key := os.Getenv("DASHSCOPE_API_KEY"); key != "" {
		clf = qwen.New(key, os.Getenv("DASHSCOPE_BASE_URL"), os.Getenv("QWEN_MODEL"), nil) // tools optional for eval
	} else {
		clf = classify.NewFake()
	}
	cases, _ := eval.LoadEmbedded()
	var rs []eval.Result
	for _, gc := range cases {
		c, err := clf.Classify(ctx, domain.Email{FromAddr: gc.From, Subject: gc.Subject, Body: gc.Body})
		if err != nil { /* count as a miss / skip with a log */ continue }
		rs = append(rs, eval.Result{ ID: gc.ID,
			ExpectedType: gc.ExpectedType, PredictedType: c.Type,
			ExpectedUrgency: gc.ExpectedUrgency, PredictedUrgency: c.Urgency,
			Confidence: c.Confidence })
	}
	fmt.Println(eval.Format(rs))
}
```
Note: `config.Load()` currently requires `DATABASE_URL`. For the eval runner either (a) read the Qwen vars directly from env (shown above, bypassing `config.Load`), or (b) relax `config` to not require DB for this path. Prefer (a) to avoid changing prod config.

- [ ] **`make eval`:**
```make
eval:
	go run ./cmd/eval
```
(`app.env` is auto-loaded by make, so `DASHSCOPE_API_KEY` is set → real Qwen.)

- [ ] **Manual run:** `make eval` with the key set → prints metrics. Record the headline numbers (e.g. "87% type accuracy, 100% recall on critical") for the demo/writeup, and set `CONFIDENCE_THRESHOLD` in `app.env` to the recommended value.

- [ ] Update `CLAUDE.md` (eval harness + `make eval`).
- [ ] Commit: `feat(eval): report, runner, and make eval`.

---

## Plan 6 Definition of Done
- [ ] `go vet ./...`, `go build ./...` clean; `go test ./...` green (metrics + dataset tested).
- [ ] `make eval` runs the gold set through the real Qwen classifier and prints accuracy, per-class P/R/F1, confusion matrix, calibration, and a recommended threshold.
- [ ] Gold dataset has ≥30 cases covering all types/urgencies incl. ambiguous ones; coverage guard test passes.
- [ ] Headline metrics captured for the submission; `CONFIDENCE_THRESHOLD` set from the recommendation.

## Next
- **Plan 7 — Deployment + submission deliverables** (`…-plan-7-deployment-submission.md`).
