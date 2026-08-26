#!/usr/bin/env bash
# Smoke test for the HatchSeal incubation inspection backend.
#
# It builds the server, starts it against a temporary SQLite database, drives a
# complete task through the public HTTP API (lock -> receipt -> start ->
# candling -> swab -> culture -> rapid -> physicochemical -> reveal -> review ->
# admit), then asserts the issued credential. No external network is used, every
# process and temporary file is cleaned up, and responses are always captured
# into variables before assertion.
set -euo pipefail

WORKDIR="$(mktemp -d)"
BIN="$WORKDIR/hatchseal-server"
DB="$WORKDIR/hatchseal.db"
PORT="${BENZHI_PORT:-18080}"
BASE="http://127.0.0.1:$PORT"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

post() {
  local path="$1"; local body="$2"
  curl -s -X POST "$BASE$path" -H 'Content-Type: application/json' -d "$body"
}

echo "==> building server"
go build -o "$BIN" ./cmd/server

echo "==> starting server on :$PORT"
"$BIN" -addr ":$PORT" -db "$DB" &
SERVER_PID=$!

echo "==> waiting for healthz"
health=""
for _ in $(seq 1 100); do
  health="$(curl -s "$BASE/healthz" || true)"
  if [[ "$health" == *'"status":"ok"'* ]]; then
    break
  fi
  sleep 0.1
done
if [[ "$health" != *'"status":"ok"'* ]]; then
  echo "FAIL: healthz never became ready (last: $health)" >&2
  exit 1
fi
echo "OK healthz: $health"

echo "==> create task"
create="$(post /v1/tasks '{"operation_id":"smoke-create","generation":1}')"
task_id="$(grep -oE '[0-9a-f]{32}' <<<"$create" || true)"
if [[ -z "$task_id" ]]; then
  echo "FAIL: create did not return a task id: $create" >&2
  exit 1
fi
echo "OK create: task_id=$task_id"

echo "==> lock task"
lock_body='{"operation_id":"smoke-lock","generation":1,"house_id":"house-1","shift_id":"shift-1","fumigation_batch_id":"fum-1","fumigation_digest":"fum-digest-0001","rule_set_version":1,"batch_no":"batch-smoke-1","incubator_slot_id":"slot-1","candling_window_id":"window-1","seals":[{"seal_no":"seal-1","positions":[1,2,3]}],"blind_codes":["blind-1","blind-2"],"culture_wells":["cw-1"],"rapid_wells":["rw-1"]}'
lock="$(post "/v1/tasks/$task_id/lock" "$lock_body")"
if [[ "$lock" != *'"status":"pending_receipt"'* ]]; then
  echo "FAIL: lock: $lock" >&2
  exit 1
fi
echo "OK lock: $lock"

echo "==> dual receipt + start"
for person in recv-1 recv-2; do
  post "/v1/tasks/$task_id/receipts" "{\"operation_id\":\"smoke-receipt-$person\",\"generation\":1,\"person_id\":\"$person\"}" >/dev/null
done
start="$(post "/v1/tasks/$task_id/start" '{"operation_id":"smoke-start","generation":1}')"
if [[ "$start" != *'"status":"resources_occupied"'* ]]; then
  echo "FAIL: start: $start" >&2
  exit 1
fi
echo "OK start"

echo "==> candling coverage"
candling="$(post "/v1/tasks/$task_id/candling" '{"operation_id":"smoke-candling","generation":1,"entries":[{"seal_no":"seal-1","position":1,"category":"fertile"},{"seal_no":"seal-1","position":2,"category":"fertile"},{"seal_no":"seal-1","position":3,"category":"infertile"}]}')"
if [[ "$candling" != *'"status":"swab_culture"'* ]]; then
  echo "FAIL: candling: $candling" >&2
  exit 1
fi
echo "OK candling"

echo "==> swab seal + culture + rapid + physicochemical"
post "/v1/tasks/$task_id/swabs/seal" '{"operation_id":"smoke-swab","generation":1,"seal_no":"seal-1"}' >/dev/null
post "/v1/tasks/$task_id/cultures/readings" '{"operation_id":"smoke-culture","generation":1,"well":"cw-1","device_id":"dev-culture"}' >/dev/null
post "/v1/tasks/$task_id/rapid-tests/readings" '{"operation_id":"smoke-rapid","generation":1,"well":"rw-1","device_id":"dev-reader"}' >/dev/null
for kind in egg_weight air_cell_height cleanliness fumigation_residue; do
  post "/v1/tasks/$task_id/physicochemical" "{\"operation_id\":\"smoke-phys-$kind\",\"generation\":1,\"seal_no\":\"seal-1\",\"position\":1,\"kind\":\"$kind\",\"device_id\":\"dev-scale\"}" >/dev/null
done

echo "==> reveal + reviews + admit"
post "/v1/tasks/$task_id/blind/reveal" '{"operation_id":"smoke-reveal","generation":1,"codes":["blind-1","blind-2"]}' >/dev/null
post "/v1/tasks/$task_id/reviews" '{"operation_id":"smoke-review-1","generation":1,"person_id":"rev-1","decision":"pass"}' >/dev/null
post "/v1/tasks/$task_id/reviews" '{"operation_id":"smoke-review-2","generation":1,"person_id":"rev-2","decision":"pass"}' >/dev/null
final="$(post "/v1/tasks/$task_id/decisions/admit" '{"operation_id":"smoke-admit","generation":1}')"
if [[ "$final" != *'"status":"admitted"'* ]]; then
  echo "FAIL: admit: $final" >&2
  exit 1
fi
echo "OK admit"

echo "==> read credential"
credential="$(curl -s "$BASE/v1/tasks/$task_id/credential")"
if [[ "$credential" != *'"number":"INC-'* ]]; then
  echo "FAIL: credential: $credential" >&2
  exit 1
fi
echo "OK credential: $credential"

echo "==> smoke test passed"
