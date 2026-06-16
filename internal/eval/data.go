// Package eval computes classification-quality metrics for the gold dataset:
// accuracy, per-class precision/recall/F1, confusion matrices, and a confidence
// threshold calibration sweep.
//
// data.go provides the shared types and the file-I/O helpers (LoadGold,
// LoadPredictions, SavePredictions). The metric-computation functions added in
// later files are pure; cmd/eval is the only entry point that calls the model.
package eval

import (
	"bufio"
	"bytes"
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
		raw := bytes.TrimSpace(sc.Bytes())
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
