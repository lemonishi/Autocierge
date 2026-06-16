// Package tools implements qwen.ToolBox over the store: the external tools the
// Qwen classifier can invoke during classification to disambiguate hard cases.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lemonishi/autocierge/internal/qwen"
	"github.com/lemonishi/autocierge/internal/store"
)

// Box is a store-backed qwen.ToolBox.
type Box struct {
	store *store.Store
}

var _ qwen.ToolBox = (*Box)(nil)

func New(s *store.Store) *Box { return &Box{store: s} }

// Definitions returns the function-calling schemas the model may invoke.
func (b *Box) Definitions() []qwen.ToolDefinition {
	return []qwen.ToolDefinition{
		{
			Name:        "lookup_customer",
			Description: "Look up a customer's account tier and status by their email address. Use this to gauge urgency/priority for known customers.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"email": map[string]any{"type": "string", "description": "the customer's email address"},
				},
				"required": []string{"email"},
			},
		},
		{
			Name:        "lookup_similar_tickets",
			Description: "Find how past support tickets matching a keyword were classified (type and urgency). Use this to stay consistent with prior classifications.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "a short keyword to match past ticket subjects/bodies, e.g. 'invoice'"},
				},
				"required": []string{"query"},
			},
		},
	}
}

// Invoke dispatches a tool call and returns a JSON string result.
func (b *Box) Invoke(ctx context.Context, name, argsJSON string) (string, error) {
	switch name {
	case "lookup_customer":
		var args struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("bad arguments for lookup_customer: %w", err)
		}
		cust, err := b.store.GetCustomer(ctx, args.Email)
		if errors.Is(err, store.ErrNotFound) {
			return `{"found":false}`, nil
		}
		if err != nil {
			return "", err
		}
		return jsonString(map[string]any{
			"found": true, "tier": cust.Tier, "account_status": cust.AccountStatus, "name": cust.Name,
		}), nil

	case "lookup_similar_tickets":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("bad arguments for lookup_similar_tickets: %w", err)
		}
		sims, err := b.store.FindSimilarTickets(ctx, args.Query, 5)
		if err != nil {
			return "", err
		}
		results := make([]map[string]any, 0, len(sims))
		for _, s := range sims {
			results = append(results, map[string]any{
				"subject": s.Subject, "type": string(s.Type), "urgency": string(s.Urgency),
			})
		}
		return jsonString(map[string]any{"count": len(results), "tickets": results}), nil

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
