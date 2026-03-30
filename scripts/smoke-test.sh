#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
PASS=0
FAIL=0

green() { printf "\033[32m%s\033[0m\n" "$*"; }
red()   { printf "\033[31m%s\033[0m\n" "$*"; }

check() {
  local name="$1" expected="$2" actual="$3"
  if [ "$actual" -eq "$expected" ]; then
    green "  [PASS] $name (HTTP $actual)"
    PASS=$((PASS + 1))
  else
    red "  [FAIL] $name — expected $expected, got $actual"
    FAIL=$((FAIL + 1))
  fi
}

echo "=== MetaChat Smoke Test ==="
echo "Base URL: $BASE_URL"
echo ""

# ── Wait for service ────────────────────────────────────────────
echo "Waiting for service to be ready..."
for i in $(seq 1 30); do
  if curl -sf "$BASE_URL/health" > /dev/null 2>&1; then
    green "Service is ready!"
    break
  fi
  if [ "$i" -eq 30 ]; then
    red "Service did not become ready in time"
    exit 1
  fi
  sleep 2
done
echo ""

TIMESTAMP=$(date +%s)
USER1_EMAIL="smoketest-user1-${TIMESTAMP}@test.com"
USER2_EMAIL="smoketest-user2-${TIMESTAMP}@test.com"

# ── 1. Health check ─────────────────────────────────────────────
echo "1. Health check"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/health")
check "GET /health" 200 "$HTTP_CODE"
echo ""

# ── 2. Register user1 ───────────────────────────────────────────
echo "2. Register user1"
REGISTER1_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$USER1_EMAIL\",\"password\":\"password123\",\"name\":\"Smoke User 1\"}")
REGISTER1_BODY=$(echo "$REGISTER1_RESP" | sed '$d')
REGISTER1_CODE=$(echo "$REGISTER1_RESP" | tail -1)
check "POST /api/v1/auth/register (user1)" 201 "$REGISTER1_CODE"
USER1_ID=$(echo "$REGISTER1_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "  User1 ID: $USER1_ID"
echo ""

# ── 3. Register user2 ───────────────────────────────────────────
echo "3. Register user2"
REGISTER2_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$USER2_EMAIL\",\"password\":\"password123\",\"name\":\"Smoke User 2\"}")
REGISTER2_BODY=$(echo "$REGISTER2_RESP" | sed '$d')
REGISTER2_CODE=$(echo "$REGISTER2_RESP" | tail -1)
check "POST /api/v1/auth/register (user2)" 201 "$REGISTER2_CODE"
USER2_ID=$(echo "$REGISTER2_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "  User2 ID: $USER2_ID"
echo ""

# ── 4. Login as user1 ───────────────────────────────────────────
echo "4. Login as user1"
LOGIN_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$USER1_EMAIL\",\"password\":\"password123\"}")
LOGIN_BODY=$(echo "$LOGIN_RESP" | sed '$d')
LOGIN_CODE=$(echo "$LOGIN_RESP" | tail -1)
check "POST /api/v1/auth/login" 200 "$LOGIN_CODE"
ACCESS_TOKEN=$(echo "$LOGIN_BODY" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)
if [ -n "$ACCESS_TOKEN" ]; then
  green "  Got access_token"
else
  red "  No access_token in response"
  FAIL=$((FAIL + 1))
fi
echo ""

# ── 5. Get profile ──────────────────────────────────────────────
echo "5. Get profile"
PROFILE_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/v1/profile" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
check "GET /api/v1/profile" 200 "$PROFILE_CODE"
echo ""

# ── 6. Create conversation ──────────────────────────────────────
echo "6. Create conversation between user1 and user2"
CONV_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/conversations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d "{\"participant_ids\":[\"$USER2_ID\"]}")
CONV_BODY=$(echo "$CONV_RESP" | sed '$d')
CONV_CODE=$(echo "$CONV_RESP" | tail -1)
check "POST /api/v1/conversations" 201 "$CONV_CODE"
CONV_ID=$(echo "$CONV_BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "  Conversation ID: $CONV_ID"
echo ""

# ── 7. Send message ─────────────────────────────────────────────
echo "7. Send message from user1"
MSG_RESP=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/v1/messages" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d "{\"conversation_id\":\"$CONV_ID\",\"content\":\"Hello from smoke test!\",\"content_type\":\"text\"}")
MSG_BODY=$(echo "$MSG_RESP" | sed '$d')
MSG_CODE=$(echo "$MSG_RESP" | tail -1)
check "POST /api/v1/messages" 201 "$MSG_CODE"
echo ""

# ── 8. Get messages ─────────────────────────────────────────────
echo "8. Get messages"
MSGS_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
  "$BASE_URL/api/v1/conversations/messages?conversation_id=$CONV_ID" \
  -H "Authorization: Bearer $ACCESS_TOKEN")
check "GET /api/v1/conversations/messages" 200 "$MSGS_CODE"
echo ""

# ── Summary ─────────────────────────────────────────────────────
echo "==============================="
echo "Results: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then
  red "SMOKE TEST FAILED"
  exit 1
else
  green "ALL SMOKE TESTS PASSED"
fi
