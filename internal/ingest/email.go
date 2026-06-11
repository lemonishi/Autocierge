// Package ingest provides email parsing and IMAP ingestion for SupportSentinel.
package ingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"mime"
	"net/mail"
	"regexp"
	"strings"

	gomessagemail "github.com/emersion/go-message/mail"

	"github.com/lemonishi/supportsentinel/internal/domain"
)

// ParseRFC822 turns a raw RFC822 email message into a normalized domain.Email.
//
// It uses net/mail for top-level header parsing and github.com/emersion/go-message/mail
// to walk MIME parts and extract the text/plain body. For non-MIME messages the
// raw body is used directly.
//
// DedupeKey is the trimmed Message-ID header when present, otherwise
// hashKey(fromAddr, subject, body) — the same sha256 scheme used by the HTTP
// adapter in internal/httpapi.
func ParseRFC822(raw []byte) (domain.Email, error) {
	// Parse headers using stdlib net/mail.
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return domain.Email{}, fmt.Errorf("ingest: read message: %w", err)
	}

	// Extract From address (address only, no display name).
	fromHeader := msg.Header.Get("From")
	fromAddr, err := parseAddress(fromHeader)
	if err != nil {
		return domain.Email{}, fmt.Errorf("ingest: parse From address %q: %w", fromHeader, err)
	}

	subject := decodeHeader(msg.Header.Get("Subject"))
	rawMsgID := strings.TrimSpace(msg.Header.Get("Message-Id"))

	// Extract the body: walk MIME parts looking for text/plain.
	// CreateReader re-parses from the full raw bytes so that go-message
	// handles MIME/multipart decoding independently of stdlib's reader.
	body, err := extractBody(raw)
	if err != nil {
		return domain.Email{}, fmt.Errorf("ingest: extract body: %w", err)
	}

	// Build DedupeKey.
	var dedupeKey string
	if rawMsgID != "" {
		dedupeKey = rawMsgID
	} else {
		dedupeKey = hashKey(fromAddr, subject, body)
	}

	// Build Raw metadata map for traceability.
	rawMeta := map[string]any{
		"message-id": rawMsgID,
		"date":       msg.Header.Get("Date"),
	}

	return domain.Email{
		FromAddr:  fromAddr,
		Subject:   subject,
		Body:      body,
		DedupeKey: dedupeKey,
		Raw:       rawMeta,
	}, nil
}

// parseAddress extracts the email address from a header value, stripping the
// display name. Falls back to the raw trimmed value if net/mail cannot parse it.
func parseAddress(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	addr, err := mail.ParseAddress(v)
	if err != nil {
		// If parsing fails, return the raw value so callers can still log it.
		return v, nil
	}
	return addr.Address, nil
}

// headerDecoder decodes RFC 2047 "encoded-word" header values (e.g.
// "=?UTF-8?q?Caf=C3=A9?=") into plain UTF-8. The default CharsetReader handles
// UTF-8, US-ASCII, and ISO-8859-1; other charsets fall back to the raw value.
var headerDecoder = &mime.WordDecoder{}

// decodeHeader returns v with any RFC 2047 encoded-words decoded. If decoding
// fails (e.g. an unsupported charset) the original trimmed value is returned so
// the ticket still carries a best-effort subject rather than an error.
func decodeHeader(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	decoded, err := headerDecoder.DecodeHeader(v)
	if err != nil {
		return v
	}
	return decoded
}

var (
	// scriptStyleRe drops <script>/<style> blocks including their contents.
	scriptStyleRe = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)
	// blockBreakRe maps block-level breaks to newlines so the text stays readable.
	blockBreakRe = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|tr|li|h[1-6]|table)>`)
	// tagRe strips any remaining HTML tags.
	tagRe = regexp.MustCompile(`(?s)<[^>]*>`)
	// inlineSpaceRe collapses runs of spaces/tabs/non-breaking spaces.
	inlineSpaceRe = regexp.MustCompile("[ \t ]+")
)

// htmlToText reduces an HTML body to readable plain text: it drops
// script/style blocks, converts block breaks to newlines, strips remaining
// tags, decodes HTML entities, and collapses redundant whitespace. This keeps
// the classifier and reviewers from seeing raw markup for HTML-only emails.
func htmlToText(s string) string {
	s = scriptStyleRe.ReplaceAllString(s, " ")
	s = blockBreakRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(inlineSpaceRe.ReplaceAllString(ln, " "))
		if ln != "" {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// extractBody walks the MIME structure of raw and returns the text/plain part.
// If the message has no text/plain but has text/html, the raw HTML is returned
// (tags not stripped — acceptable for ticket body storage). For completely
// non-MIME messages the body is returned as-is.
func extractBody(raw []byte) (string, error) {
	mr, err := gomessagemail.CreateReader(bytes.NewReader(raw))
	if err != nil && !isUnknownCharsetErr(err) {
		// Fall back to reading without MIME parsing.
		return bodyFallback(raw), nil
	}
	defer mr.Close()

	var plainText string
	var htmlText string

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil && !isUnknownCharsetErr(err) {
			break
		}
		if p == nil {
			continue
		}

		inline, ok := p.Header.(*gomessagemail.InlineHeader)
		if !ok {
			// Attachment — skip.
			continue
		}

		ct, _, _ := inline.ContentType()
		bodyBytes, readErr := io.ReadAll(p.Body)
		if readErr != nil {
			continue
		}
		text := string(bodyBytes)

		switch {
		case strings.HasPrefix(ct, "text/plain"):
			if plainText == "" {
				plainText = text
			}
		case strings.HasPrefix(ct, "text/html"):
			if htmlText == "" {
				htmlText = text
			}
		}
	}

	if plainText != "" {
		return plainText, nil
	}
	if htmlText != "" {
		return htmlToText(htmlText), nil
	}
	// Nothing usable from MIME walk — fall back to raw body.
	return bodyFallback(raw), nil
}

// bodyFallback returns the raw message body (everything after the first blank
// line) as a string, for non-MIME or unparseable messages.
func bodyFallback(raw []byte) string {
	// Find the blank line separator between headers and body.
	sep := []byte("\r\n\r\n")
	idx := bytes.Index(raw, sep)
	if idx == -1 {
		sep = []byte("\n\n")
		idx = bytes.Index(raw, sep)
	}
	if idx == -1 {
		return string(raw)
	}
	return string(raw[idx+len(sep):])
}

// isUnknownCharsetErr reports whether err is a go-message unknown-charset
// error. go-message exports message.IsUnknownCharset; we import it via the
// top-level message package alias.
func isUnknownCharsetErr(err error) bool {
	// go-message wraps these as "charset" errors; check message string as a
	// lightweight guard — the real check is message.IsUnknownCharset but we
	// avoid importing the top-level package just for this.
	return err != nil && strings.Contains(err.Error(), "charset")
}

// hashKey computes sha256 of parts joined with a 0x00 byte separator and
// returns the hex-encoded digest. This is the same scheme used by
// internal/httpapi for HTTP-adapter dedupe keys.
func hashKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
