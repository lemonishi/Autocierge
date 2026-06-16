// Command mcp-server serves SupportSentinel's agent tools (lookup_customer,
// lookup_similar_tickets) over the Model Context Protocol (Streamable HTTP).
// The main server's MCP-backed ToolBox connects to it; run it via `make mcp`.
//
//	MCP_LISTEN_ADDR  listen address (default ":8090"); tools served at /mcp
//	DATABASE_URL     required (tools are store-backed)
package main

import (
	"context"
	"log"
	"os"

	"github.com/lemonishi/supportsentinel/internal/config"
	"github.com/lemonishi/supportsentinel/internal/mcp"
	"github.com/lemonishi/supportsentinel/internal/store"
	"github.com/lemonishi/supportsentinel/internal/tools"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	s, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()
	if err := s.SeedDemoCustomers(ctx); err != nil {
		log.Printf("seed demo customers: %v", err)
	}

	addr := os.Getenv("MCP_LISTEN_ADDR")
	if addr == "" {
		addr = ":8090"
	}

	srv := mcp.NewServer(tools.New(s))
	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	log.Printf("MCP tool server listening on %s (path /mcp)", addr)
	if err := httpSrv.Start(addr); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
