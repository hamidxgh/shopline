#!/usr/bin/env bash
# End-to-end prove: Keycloak token -> ensure product -> POST /orders -> ClickHouse row.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

EDGE="${EDGE_URL:-http://localhost:8082}"
KEYCLOAK="${KEYCLOAK_URL:-http://localhost:8084}"
CH="${CLICKHOUSE_URL:-http://localhost:8123}"

echo "==> health via nginx"
code=$(curl -sS -o /dev/null -w '%{http_code}' http://localhost:8080/healthz || true)
if [[ "$code" != "200" ]]; then
  echo "FAIL: /healthz returned ${code:-none} (is the stack up? make up)"
  exit 1
fi
echo "ok ($code)"

echo "==> ensure ClickHouse order_events"
curl -sS "$CH/?user=shopline&password=shopline&database=shopline" \
  --data-binary @deploy/clickhouse/init.sql >/dev/null || true

echo "==> token from Keycloak"
TOKEN=$(curl -sS -X POST "$KEYCLOAK/realms/shopline/protocol/openid-connect/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=password&client_id=shopline-api&username=shopper&password=shopper' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')
echo "token_len=${#TOKEN}"

echo "==> ensure product id=1 exists (create if GET fails)"
if ! curl -sS -f -H "Authorization: Bearer $TOKEN" "$EDGE/products/1" >/dev/null 2>&1; then
  curl -sS -X POST "$EDGE/products" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d '{"name":"Smoke Tee","price_cents":777,"stock":100}'
  echo
fi

echo "==> create order via edge ($EDGE)"
ORDER_JSON=$(curl -sS -X POST "$EDGE/orders" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"items":[{"product_id":1,"quantity":1}]}')
echo "$ORDER_JSON"
ORDER_ID=$(python3 -c 'import sys,json; print(json.load(sys.stdin)["order_id"])' <<<"$ORDER_JSON")

echo "==> ClickHouse order_events for order_id=$ORDER_ID"
ROWS=$(curl -sS "$CH/?user=shopline&password=shopline&database=shopline" \
  --data-binary "SELECT count() FROM order_events WHERE order_id = ${ORDER_ID}")
echo "rows=$ROWS"
if [[ "${ROWS//[$'\t\r\n']}" -lt 1 ]]; then
  echo "FAIL: expected at least one analytics row"
  exit 1
fi

echo "SMOKE OK"
