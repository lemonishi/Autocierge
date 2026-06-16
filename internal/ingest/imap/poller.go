// Package imap provides a background poller that watches an IMAP mailbox and
// feeds unseen messages into the orchestrator via the shared Ingest path.
//
// The poller is thin glue: it relies on ingest.ParseRFC822 for parsing and on
// the orchestrator's idempotent Ingest (dedupe via Message-ID) for safety. It
// depends only on the Ingestor interface, never on the orchestrator package.
package imap

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/lemonishi/autocierge/internal/config"
	"github.com/lemonishi/autocierge/internal/domain"
	"github.com/lemonishi/autocierge/internal/ingest"
)

// Ingestor is the seam the poller depends on. The orchestrator satisfies it
// via Ingest(ctx, source, e) (domain.Ticket, error).
type Ingestor interface {
	Ingest(ctx context.Context, source string, e domain.Email) (domain.Ticket, error)
}

// Poller watches an IMAP mailbox and feeds unseen messages into the orchestrator.
type Poller struct {
	cfg   config.Config
	ing   Ingestor
	every time.Duration
}

// New builds a Poller. The poll interval is derived from cfg.IMAPPollSeconds.
func New(cfg config.Config, ing Ingestor) *Poller {
	return &Poller{
		cfg:   cfg,
		ing:   ing,
		every: time.Duration(cfg.IMAPPollSeconds) * time.Second,
	}
}

// Run blocks, polling until ctx is cancelled. Best-effort: it logs errors from a
// poll cycle and retries on the next tick rather than aborting.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.every)
	defer t.Stop()
	for {
		if err := p.pollOnce(ctx); err != nil {
			log.Printf("imap poll: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// pollOnce dials TLS to the IMAP server, logs in, selects the mailbox, searches
// for UNSEEN messages, and for each: fetches the raw RFC822 bytes, parses them
// via ingest.ParseRFC822, ingests via p.ing.Ingest (idempotent on DedupeKey),
// then marks the message \Seen. Per-message errors are logged and skipped so a
// single bad message never blocks the rest of the batch.
func (p *Poller) pollOnce(ctx context.Context) error {
	addr := p.cfg.IMAPHost + ":" + p.cfg.IMAPPort
	c, err := imapclient.DialTLS(addr, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer c.Close()

	if err := c.Login(p.cfg.IMAPUsername, p.cfg.IMAPPassword).Wait(); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = c.Logout().Wait() }()

	if _, err := c.Select(p.cfg.IMAPMailbox, nil).Wait(); err != nil {
		return fmt.Errorf("select %s: %w", p.cfg.IMAPMailbox, err)
	}

	criteria := &imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}
	searchData, err := c.Search(criteria, nil).Wait()
	if err != nil {
		return fmt.Errorf("search unseen: %w", err)
	}

	seqNums := searchData.AllSeqNums()
	if len(seqNums) == 0 {
		return nil
	}

	for _, seqNum := range seqNums {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.handleMessage(ctx, c, seqNum); err != nil {
			log.Printf("imap poll: message seq %d: %v", seqNum, err)
			continue
		}
	}
	return nil
}

// handleMessage fetches a single message's raw bytes, ingests it, and marks it
// \Seen. Marking \Seen only happens after a successful Ingest, so a crash
// between ingest and flag-set is safe (re-ingest dedupes on Message-ID).
func (p *Poller) handleMessage(ctx context.Context, c *imapclient.Client, seqNum uint32) error {
	seqSet := imap.SeqSetNum(seqNum)
	bodySection := &imap.FetchItemBodySection{}
	fetchOpts := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}

	msgs, err := c.Fetch(seqSet, fetchOpts).Collect()
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	if len(msgs) == 0 {
		return fmt.Errorf("fetch returned no message")
	}

	raw := msgs[0].FindBodySection(bodySection)
	if len(raw) == 0 {
		return fmt.Errorf("empty body section")
	}

	email, err := ingest.ParseRFC822(raw)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if _, err := p.ing.Ingest(ctx, "imap", email); err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	storeFlags := &imap.StoreFlags{
		Op:    imap.StoreFlagsAdd,
		Flags: []imap.Flag{imap.FlagSeen},
	}
	if err := c.Store(seqSet, storeFlags, nil).Close(); err != nil {
		return fmt.Errorf("mark seen: %w", err)
	}
	return nil
}
