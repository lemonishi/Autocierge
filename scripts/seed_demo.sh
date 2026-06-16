#!/usr/bin/env bash
# Seed the dashboard with a spread of representative support emails so the demo
# isn't empty. Posts to the running server's ingest endpoint.
#
#   BASE_URL=http://localhost:8080 ./scripts/seed_demo.sh
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:8080}"

# JSON-escape a string via python3 (no jq dependency).
json() { printf '%s' "$1" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))'; }

post() {
  local from="$1" subject="$2" body="$3"
  curl -fsS -X POST "${BASE_URL}/api/emails" \
    -H 'content-type: application/json' \
    -d "$(printf '{"from":%s,"subject":%s,"body":%s}' \
          "$(json "$from")" "$(json "$subject")" "$(json "$body")")" >/dev/null
  echo "  seeded: ${subject}"
}

echo "==> seeding demo tickets to ${BASE_URL}"
post "frank@acme.com" "Production API returning 500 for all calls" "Every request has 500'd for 20 minutes. Checkout is completely down."
post "kate@acme.com"  "Cannot log in - password reset never arrives" "I requested a reset three times, no email. Locked out of my account."
post "alice@acme.com" "Charged twice for my subscription"            "I was billed \$49 twice this month. Please refund the duplicate."
post "pat@acme.com"   "Feature request: dark mode"                   "A dark theme for the dashboard would be great for late-night work."
post "bea@acme.com"   "Security: possible unauthorized access"       "I see logins from a country we do not operate in. Please lock it down now."
post "wes@acme.com"   "Thanks for the great product"                 "Just wanted to say the team loves the new release."
echo "==> done — open ${BASE_URL} to review the queue"
