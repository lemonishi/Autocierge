// Command eval runs the gold dataset through the classifier and prints a quality
// report: accuracy, per-class precision/recall/F1, confusion matrices, and a
// confidence-threshold calibration sweep.
//
// Run from the repository root so the default eval/ paths resolve:
//
//	go run ./cmd/eval            # replay the committed cache (eval/recorded.json)
//	go run ./cmd/eval --live     # classify with real Qwen, refresh the cache
//	go run ./cmd/eval --fake     # classify with the deterministic fake, refresh the cache
//
// Note: `go build ./cmd/eval` fails because the default output name "eval"
// collides with the eval/ data directory. Use `go run ./cmd/eval` (as `make eval`
// does) or `go build -o bin/eval ./cmd/eval`.
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
		if os.Getenv("DASHSCOPE_API_KEY") == "" {
			log.Fatal("eval: --live requires DASHSCOPE_API_KEY")
		}
		clf := qwen.New(
			os.Getenv("DASHSCOPE_API_KEY"),
			getenv("DASHSCOPE_BASE_URL", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"),
			getenv("QWEN_MODEL", "qwen-max"),
			nil,
		)
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
