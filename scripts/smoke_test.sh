#!/usr/bin/env bash
# smoke_test.sh — 真实启动服务、轮询健康检查、发报文、断重复、清理。
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$HERE"

PORT="${SMOKE_PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"
BIN="$(mktemp -d)/server"
DB_CONTAINER="dtel-smoke-pg"
DB_PORT="${SMOKE_DB_PORT:-15432}"

cleanup() {
  echo "[smoke] cleaning up"
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if docker ps -a --format '{{.Names}}' | grep -qx "$DB_CONTAINER"; then
    docker rm -f "$DB_CONTAINER" >/dev/null 2>&1 || true
  fi
  if [[ -n "${TMPDIR:-}" ]]; then rm -rf "$TMPDIR" 2>/dev/null || true; fi
}
trap cleanup EXIT

echo "[smoke] building server"
go build -o "$BIN" ./cmd/server

echo "[smoke] starting postgres (container $DB_CONTAINER)"
if ! docker ps -a --format '{{.Names}}' | grep -qx "$DB_CONTAINER"; then
  docker run -d --name "$DB_CONTAINER" \
    -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=telemetry \
    -p "${DB_PORT}:5432" postgres:16-alpine >/dev/null
fi

# Wait for postgres to accept connections.
for i in $(seq 1 30); do
  if docker exec "$DB_CONTAINER" pg_isready -U postgres >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

DATABASE_URL="postgres://postgres:postgres@127.0.0.1:${DB_PORT}/telemetry?sslmode=disable"
PORT="$PORT" DATABASE_URL="$DATABASE_URL" "$BIN" &
SERVER_PID=$!

echo "[smoke] waiting for healthz"
for i in $(seq 1 30); do
  if CODE=$(curl -s -o /tmp/dtel_health.json -w '%{http_code}' "$BASE/healthz" 2>/dev/null) && [[ "$CODE" == "200" ]]; then
    break
  fi
  sleep 1
done

CODE=$(curl -s -o /tmp/dtel_health.json -w '%{http_code}' "$BASE/healthz")
if [[ "$CODE" != "200" ]]; then
  echo "[smoke] FAIL healthz returned $CODE" >&2
  exit 1
fi
if ! grep -q '"alive"' /tmp/dtel_health.json; then
  echo "[smoke] FAIL healthz body missing alive" >&2
  exit 1
fi
echo "[smoke] healthz OK"

echo "[smoke] creating device"
DEV_CODE=$(curl -s -o /tmp/dtel_dev.json -w '%{http_code}' -X POST "$BASE/api/v1/devices" \
  -H 'Content-Type: application/json' \
  -d '{"device_id":"SMOKETEST1","name":"smoke","protocol_version":"v1","metadata":{}}')
if [[ "$DEV_CODE" != "201" ]]; then
  echo "[smoke] FAIL create device returned $DEV_CODE" >&2
  exit 1
fi

echo "[smoke] creating rule"
curl -s -X POST "$BASE/api/v1/rules" -H 'Content-Type: application/json' \
  -d '{"rule_id":"smoke-rule","name":"smoke rule","matcher":{"metrics":["temperature"]},"topic":"sensors/temp","priority":1,"enabled":true}' >/dev/null

# Build a valid protocol message using a small helper via the running server is
# not available; construct manually with a known body and CRC16 computed here.
build_message() {
  # device|ts|metric|value|unit  (body) then |CHECKSUM
  local body="$1"
  local crc=65535
  local i ch
  for ((i=0; i<${#body}; i++)); do
    ch=$(printf '%d' "'${body:$i:1}")
    crc=$(( (crc ^ (ch << 8)) & 0xFFFF ))
    for ((j=0; j<8; j++)); do
      if (( (crc & 0x8000) != 0 )); then
        crc=$(( ((crc << 1) ^ 0x1021) & 0xFFFF ))
      else
        crc=$(( (crc << 1) & 0xFFFF ))
      fi
    done
  done
  printf '%s|%04X' "$body" "$crc"
}

TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
RAW=$(build_message "SMOKETEST1|${TS}|temperature|25.5|C")
echo "[smoke] raw message: $RAW"

echo "[smoke] reporting telemetry"
TEL_CODE=$(curl -s -o /tmp/dtel_tel.json -w '%{http_code}' -X POST "$BASE/api/v1/telemetry" \
  -H 'Content-Type: application/json' -d "{\"raw_text\":\"$RAW\"}")
if [[ "$TEL_CODE" != "200" && "$TEL_CODE" != "201" ]]; then
  echo "[smoke] FAIL telemetry returned $TEL_CODE" >&2
  cat /tmp/dtel_tel.json >&2
  exit 1
fi
if ! grep -q '"event_id"' /tmp/dtel_tel.json; then
  echo "[smoke] FAIL no event_id in response" >&2
  exit 1
fi
echo "[smoke] telemetry OK: $(cat /tmp/dtel_tel.json)"

echo "[smoke] duplicate telemetry"
DUP_CODE=$(curl -s -o /tmp/dtel_dup.json -w '%{http_code}' -X POST "$BASE/api/v1/telemetry" \
  -H 'Content-Type: application/json' -d "{\"raw_text\":\"$RAW\"}")
if [[ "$DUP_CODE" != "200" ]]; then
  echo "[smoke] FAIL duplicate returned $DUP_CODE" >&2
  exit 1
fi
if ! grep -q '"duplicate":true' /tmp/dtel_dup.json; then
  echo "[smoke] FAIL duplicate flag not set" >&2
  exit 1
fi
echo "[smoke] duplicate OK"

# Extract event_id from the first telemetry response.
EVENT_ID=$(grep -o '"event_id":"[^"]*"' /tmp/dtel_tel.json | head -1 | sed 's/"event_id":"//;s/"//')

echo "[smoke] replay event"
REPLAY_CODE=$(curl -s -o /tmp/dtel_replay.json -w '%{http_code}' -X POST "$BASE/api/v1/events/${EVENT_ID}/replay")
if [[ "$REPLAY_CODE" != "200" ]]; then
  echo "[smoke] FAIL replay returned $REPLAY_CODE" >&2
  exit 1
fi

echo "[smoke] list deliveries"
DEL_CODE=$(curl -s -o /tmp/dtel_del.json -w '%{http_code}' "$BASE/api/v1/events/${EVENT_ID}/deliveries")
if [[ "$DEL_CODE" != "200" ]]; then
  echo "[smoke] FAIL deliveries returned $DEL_CODE" >&2
  exit 1
fi

echo "[smoke] stats"
curl -s -o /tmp/dtel_stats.json "$BASE/api/v1/stats"
if ! grep -q '"devices"' /tmp/dtel_stats.json; then
  echo "[smoke] FAIL stats missing devices" >&2
  exit 1
fi

echo "[smoke] all checks passed"
