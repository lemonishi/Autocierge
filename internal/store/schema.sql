CREATE TABLE IF NOT EXISTS tickets (
  id          UUID PRIMARY KEY,
  state       TEXT NOT NULL,
  source      TEXT NOT NULL,
  urgency     TEXT,
  type        TEXT,
  department  TEXT,
  confidence  NUMERIC,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS emails (
  id          UUID PRIMARY KEY,
  ticket_id   UUID NOT NULL REFERENCES tickets(id),
  from_addr   TEXT NOT NULL,
  to_addr     TEXT,
  subject     TEXT,
  body        TEXT,
  raw         JSONB,
  dedupe_key  TEXT UNIQUE,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS classifications (
  id         UUID PRIMARY KEY,
  ticket_id  UUID NOT NULL REFERENCES tickets(id),
  urgency    TEXT,
  type       TEXT,
  department TEXT,
  confidence NUMERIC,
  reasoning  TEXT,
  tools_used JSONB,
  model      TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS replies (
  id         UUID PRIMARY KEY,
  ticket_id  UUID NOT NULL REFERENCES tickets(id),
  draft_text TEXT,
  final_text TEXT,
  status     TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_log (
  id         UUID PRIMARY KEY,
  ticket_id  UUID NOT NULL REFERENCES tickets(id),
  from_state TEXT,
  to_state   TEXT NOT NULL,
  actor      TEXT NOT NULL,
  payload    JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS customers (
  email          TEXT PRIMARY KEY,
  name           TEXT,
  tier           TEXT,
  account_status TEXT
);

CREATE INDEX IF NOT EXISTS idx_emails_ticket_id ON emails(ticket_id);
CREATE INDEX IF NOT EXISTS idx_classifications_ticket_id ON classifications(ticket_id);
CREATE INDEX IF NOT EXISTS idx_replies_ticket_id ON replies(ticket_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_ticket_id ON audit_log(ticket_id);
