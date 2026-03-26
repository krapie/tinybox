#!/usr/bin/env bash
# e2e_test.sh — end-to-end test for tinykube + tinydns + tinyenvoy
#
# Tests:
#   1. tinykube deploys 3 whoami pods (reconciliation loop)
#   2. All 3 pods reach Running status (readiness probes)
#   3. Service resource created; endpoint API returns 3 addresses
#   4. tinydns resolves whoami DNS name to pod IPs (syncer integration)
#   5. tinyenvoy starts with dynamic endpoint discovery
#   6. Round-robin routes across all 3 pods
#   7. Prometheus metrics are recorded
#   8. Rolling update: image change replaces all pods; DNS + discovery re-sync
#   9. Delete cleans up all pods and service
#
# Prerequisites: Docker Desktop, Go 1.23+, dig (macOS default / bind-utils)
# Run from: tinybox/sample/

set -euo pipefail

TINYKUBE_DIR="../tinykube"
TINYENVOY_DIR="../tinyenvoy"
TINYDNS_DIR="../tinydns"
TKCTL_BIN="/tmp/tkctl-e2e"
TINYDNS_BIN="/tmp/tinydns-e2e"
TINYENVOY_BIN="/tmp/tinyenvoy-e2e"
TINYKUBE_API="http://localhost:8080"
ENVOY_PROXY="http://localhost:8888"
ENVOY_ADMIN="http://localhost:9090"
ENVOY_CONFIG="/tmp/tinybox-e2e-envoy.yaml"
DNS_ADDR="127.0.0.1"
DNS_PORT="5353"
LOG_TINYKUBE="/tmp/tinykube-e2e.log"
LOG_TINYDNS="/tmp/tinydns-e2e.log"
LOG_TINYENVOY="/tmp/tinyenvoy-e2e.log"

PASS=0
FAIL=0
TINYKUBE_PID=0
TINYDNS_PID=0
TINYENVOY_PID=0

pass() { echo "  ✓ $1"; PASS=$((PASS+1)); }
fail() { echo "  ✗ $1"; FAIL=$((FAIL+1)); }
section() { echo; echo "=== $1 ==="; }

cleanup() {
  echo
  echo "=== cleanup ==="
  [ "$TINYKUBE_PID"  -ne 0 ] && kill "$TINYKUBE_PID"  2>/dev/null || true
  [ "$TINYDNS_PID"   -ne 0 ] && kill "$TINYDNS_PID"   2>/dev/null || true
  [ "$TINYENVOY_PID" -ne 0 ] && kill "$TINYENVOY_PID" 2>/dev/null || true
  docker ps -q --filter "label=tinykube=true" | xargs docker rm -f 2>/dev/null || true
  echo "  done"
}
trap cleanup EXIT

# ── 0. Build tools ─────────────────────────────────────────────────────────────
section "0. Build"

echo "  building tkctl..."
if (cd "$TINYKUBE_DIR" && go build -o "$TKCTL_BIN" ./cmd/tkctl/); then
  pass "tkctl built"
else
  fail "tkctl build failed"; exit 1
fi

echo "  building tinydns..."
if (cd "$TINYDNS_DIR" && go build -o "$TINYDNS_BIN" ./cmd/tinydns/); then
  pass "tinydns built"
else
  fail "tinydns build failed"; exit 1
fi

echo "  building tinyenvoy..."
if (cd "$TINYENVOY_DIR" && go build -o "$TINYENVOY_BIN" ./cmd/envoy/); then
  pass "tinyenvoy built"
else
  fail "tinyenvoy build failed"; exit 1
fi

# ── 1. Start tinykube ──────────────────────────────────────────────────────────
section "1. Start tinykube"

lsof -ti :8080 | xargs kill -9 2>/dev/null || true
sleep 0.5

(cd "$TINYKUBE_DIR" && go run ./cmd/tinykube/ > "$LOG_TINYKUBE" 2>&1) &
TINYKUBE_PID=$!

for i in {1..15}; do
  if curl -sf "$TINYKUBE_API/apis/apps/v1/namespaces/default/deployments" > /dev/null 2>&1; then
    pass "tinykube API ready"
    break
  fi
  sleep 1
  if [ "$i" -eq 15 ]; then fail "tinykube did not start in time"; exit 1; fi
done

# ── 2. Deploy whoami ───────────────────────────────────────────────────────────
section "2. Deploy whoami (3 replicas)"

OUT=$("$TKCTL_BIN" apply -f manifests/whoami.yaml --server "$TINYKUBE_API" 2>&1)
echo "$OUT" | grep -q "created" && pass "deployment created" || fail "deployment create failed: $OUT"

echo "  waiting for pods to be Running..."
for i in {1..30}; do
  RUNNING=$("$TKCTL_BIN" get pods --server "$TINYKUBE_API" 2>/dev/null | grep -c "Running" || true)
  [ "$RUNNING" -ge 3 ] && break
  sleep 2
done

RUNNING=$("$TKCTL_BIN" get pods --server "$TINYKUBE_API" 2>/dev/null | grep -c "Running" || true)
[ "$RUNNING" -ge 3 ] && pass "3 pods Running" || fail "only $RUNNING pods Running (want 3)"

# ── 3. Service resource + endpoint API ────────────────────────────────────────
section "3. Service resource + endpoint discovery API"

OUT=$("$TKCTL_BIN" apply -f manifests/whoami-svc.yaml --server "$TINYKUBE_API" 2>&1)
echo "$OUT" | grep -q "created\|updated" && pass "service created via tkctl" \
  || fail "service create failed: $OUT"

SVC_LIST=$("$TKCTL_BIN" get services --server "$TINYKUBE_API" 2>&1)
echo "$SVC_LIST" | grep -q "whoami" && pass "tkctl get services shows whoami" \
  || fail "tkctl get services missing whoami: $SVC_LIST"

echo "  waiting for endpoint API to return 3 endpoints..."
for i in {1..15}; do
  EP_COUNT=$(curl -sf "$TINYKUBE_API/apis/v1/namespaces/default/services/whoami/endpoints" \
    | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)
  [ "${EP_COUNT:-0}" -ge 3 ] && break
  sleep 2
done

EP_COUNT=$(curl -sf "$TINYKUBE_API/apis/v1/namespaces/default/services/whoami/endpoints" \
  | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)
[ "${EP_COUNT:-0}" -ge 3 ] && pass "endpoint API returns $EP_COUNT endpoints (want ≥3)" \
  || fail "endpoint API returned ${EP_COUNT:-0} endpoints (want ≥3)"

EP_ADDRS=$(curl -sf "$TINYKUBE_API/apis/v1/namespaces/default/services/whoami/endpoints" \
  | python3 -c "import json,sys; [print(ep['addr']) for ep in json.load(sys.stdin)]" 2>/dev/null || true)
echo "$EP_ADDRS" | grep -q "^localhost:" \
  && pass "endpoint addrs are localhost:{port} (host-mapped, macOS)" \
  || fail "endpoint addrs malformed: $EP_ADDRS"

# ── 4. Start tinydns + DNS resolution ─────────────────────────────────────────
section "4. Start tinydns (tinykube syncer)"

lsof -ti :5353 | xargs kill -9 2>/dev/null || true
lsof -ti :8181 | xargs kill -9 2>/dev/null || true
lsof -ti :9053 | xargs kill -9 2>/dev/null || true
sleep 0.5

"$TINYDNS_BIN" -tinykube "$TINYKUBE_API" -namespace default > "$LOG_TINYDNS" 2>&1 &
TINYDNS_PID=$!

# Syncer polls every 10s; wait up to 30s for first sync + DNS resolution
echo "  waiting for tinydns to sync and resolve whoami..."
for i in {1..30}; do
  DNS_COUNT=$(dig "@$DNS_ADDR" -p "$DNS_PORT" whoami.default.svc.cluster.local. A +short 2>/dev/null \
    | grep -c "^[0-9]" || true)
  [ "${DNS_COUNT:-0}" -ge 1 ] && break
  sleep 1
done

DNS_IPS=$(dig "@$DNS_ADDR" -p "$DNS_PORT" whoami.default.svc.cluster.local. A +short 2>/dev/null \
  | grep "^[0-9]" || true)
DNS_COUNT=$(echo "$DNS_IPS" | grep -c "^[0-9]" 2>/dev/null || true)

[ "${DNS_COUNT:-0}" -ge 3 ] \
  && pass "DNS resolves whoami.default.svc.cluster.local. to $DNS_COUNT IPs (want ≥3)" \
  || fail "DNS returned ${DNS_COUNT:-0} IPs (want ≥3)"

# DNS uses pod container IPs (172.x.x.x), not localhost host-mapped ports
echo "$DNS_IPS" | grep -q "^172\." \
  && pass "DNS returns container IPs (172.x.x.x) for pod-to-pod communication" \
  || fail "DNS IPs not in expected container range: $DNS_IPS"

# Health endpoint
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8181/health 2>/dev/null || echo "000")
[ "$HTTP_STATUS" = "200" ] \
  && pass "tinydns health endpoint returns 200" \
  || fail "tinydns health endpoint returned $HTTP_STATUS (want 200)"

# NXDOMAIN for unknown name
NXDOMAIN=$(dig "@$DNS_ADDR" -p "$DNS_PORT" unknown.default.svc.cluster.local. A 2>/dev/null \
  | grep -c "NXDOMAIN" || true)
[ "${NXDOMAIN:-0}" -ge 1 ] \
  && pass "unknown name returns NXDOMAIN" \
  || fail "unknown name did not return NXDOMAIN"

# ── 5. Start tinyenvoy with discovery config ───────────────────────────────────
section "5. Start tinyenvoy (discovery mode)"

lsof -ti :8888 | xargs kill -9 2>/dev/null || true
lsof -ti :9090 | xargs kill -9 2>/dev/null || true
sleep 0.5

cat > "$ENVOY_CONFIG" <<EOF
listener:
  addr: ":8888"
  tls:
    enabled: false
admin:
  addr: ":9090"
clusters:
  - name: whoami
    lb_policy: round-robin
    health_check:
      path: /health
      interval: 5s
      timeout: 2s
      unhealthy_threshold: 3
      healthy_threshold: 2
    discovery:
      tinykube_addr: $TINYKUBE_API
      service: whoami
      namespace: default
      interval: 3s
routes:
  - virtual_host: "*"
    routes:
      - prefix: /
        cluster: whoami
EOF

"$TINYENVOY_BIN" -config "$ENVOY_CONFIG" > "$LOG_TINYENVOY" 2>&1 &
TINYENVOY_PID=$!

echo "  waiting for tinyenvoy proxy to be ready..."
for i in {1..15}; do
  if curl -sf "$ENVOY_PROXY/" > /dev/null 2>&1; then
    pass "tinyenvoy proxy ready"
    break
  fi
  sleep 1
  if [ "$i" -eq 15 ]; then fail "tinyenvoy did not start in time"; exit 1; fi
done

# ── 6. Round-robin routing ─────────────────────────────────────────────────────
section "6. Round-robin routing"

HOSTNAMES=""
for i in {1..9}; do
  H=$(curl -sf "$ENVOY_PROXY/" | grep "^Hostname:" | awk '{print $2}')
  HOSTNAMES="$HOSTNAMES $H"
done

UNIQUE=$(echo "$HOSTNAMES" | tr ' ' '\n' | sort -u | grep -v '^$' | wc -l | tr -d ' ')
[ "$UNIQUE" -ge 3 ] && pass "round-robin hit $UNIQUE distinct backends" \
  || fail "round-robin only hit $UNIQUE backends (want ≥3)"

# ── 7. Prometheus metrics ──────────────────────────────────────────────────────
section "7. Prometheus metrics"

METRICS=$(curl -sf "$ENVOY_ADMIN/metrics")

echo "$METRICS" | grep -q "tinyenvoy_requests_total" \
  && pass "tinyenvoy_requests_total present" || fail "tinyenvoy_requests_total missing"
echo "$METRICS" | grep -q "tinyenvoy_request_duration_seconds" \
  && pass "tinyenvoy_request_duration_seconds present" || fail "tinyenvoy_request_duration_seconds missing"

REQ_COUNT=$(echo "$METRICS" | grep 'tinyenvoy_requests_total{.*status="200"' \
  | awk '{print $2}' | head -1)
[ "${REQ_COUNT:-0}" -ge 9 ] \
  && pass "request counter ≥9 (got $REQ_COUNT)" || fail "request counter too low (got ${REQ_COUNT:-0})"

# ── 8. Rolling update ──────────────────────────────────────────────────────────
section "8. Rolling update"

"$TKCTL_BIN" apply --name whoami --image traefik/whoami:v1.10 --replicas 3 --port 80 \
  --server "$TINYKUBE_API" > /dev/null
pass "rolling update triggered"

echo "  waiting for pods to flip to v1.10..."
for i in {1..30}; do
  NEW=$("$TKCTL_BIN" get pods --server "$TINYKUBE_API" 2>/dev/null | grep -c "v1.10" || true)
  [ "$NEW" -ge 3 ] && break
  sleep 3
done

NEW=$("$TKCTL_BIN" get pods --server "$TINYKUBE_API" 2>/dev/null | grep -c "v1.10" || true)
[ "$NEW" -ge 3 ] && pass "rolling update complete — $NEW pods on v1.10" \
  || fail "rolling update incomplete — only $NEW pods on v1.10"

# Endpoint API re-syncs after rolling update
echo "  waiting for endpoint API to reflect new pods..."
for i in {1..15}; do
  EP_COUNT=$(curl -sf "$TINYKUBE_API/apis/v1/namespaces/default/services/whoami/endpoints" \
    | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)
  [ "${EP_COUNT:-0}" -ge 3 ] && break
  sleep 2
done

EP_AFTER=$(curl -sf "$TINYKUBE_API/apis/v1/namespaces/default/services/whoami/endpoints" \
  | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0)
[ "${EP_AFTER:-0}" -ge 3 ] && pass "endpoint API returns $EP_AFTER endpoints after rolling update" \
  || fail "endpoint API returned ${EP_AFTER:-0} endpoints after rolling update (want ≥3)"

# tinydns re-syncs after rolling update (new pod IPs from new containers)
echo "  waiting for tinydns to re-sync new pod IPs..."
for i in {1..20}; do
  DNS_COUNT=$(dig "@$DNS_ADDR" -p "$DNS_PORT" whoami.default.svc.cluster.local. A +short 2>/dev/null \
    | grep -c "^[0-9]" || true)
  [ "${DNS_COUNT:-0}" -ge 3 ] && break
  sleep 2
done

DNS_COUNT=$(dig "@$DNS_ADDR" -p "$DNS_PORT" whoami.default.svc.cluster.local. A +short 2>/dev/null \
  | grep -c "^[0-9]" || true)
[ "${DNS_COUNT:-0}" -ge 3 ] && pass "DNS still resolves $DNS_COUNT IPs after rolling update" \
  || fail "DNS returned ${DNS_COUNT:-0} IPs after rolling update (want ≥3)"

# ── 9. Delete cleans up pods and service ──────────────────────────────────────
section "9. Delete deployment + service"

OUT=$("$TKCTL_BIN" delete deployment whoami --server "$TINYKUBE_API" 2>&1)
echo "$OUT" | grep -q "deleted" && pass "deployment deleted" || fail "deployment delete failed: $OUT"

OUT=$("$TKCTL_BIN" delete service whoami --server "$TINYKUBE_API" 2>&1)
echo "$OUT" | grep -q "deleted" && pass "service deleted via tkctl" || fail "service delete failed: $OUT"

echo "  waiting for reconcile to remove pods..."
sleep 10

PODS=$("$TKCTL_BIN" get pods --server "$TINYKUBE_API" 2>/dev/null \
  | grep -v "^NAME" | grep -vc "^$" || true)
[ "${PODS:-0}" -eq 0 ] && pass "all pods removed after delete" \
  || fail "${PODS:-0} pods still exist after delete"

CONTAINERS=$(docker ps -q --filter "label=tinykube=true" | wc -l | tr -d ' ' || true)
[ "$CONTAINERS" -eq 0 ] && pass "all Docker containers removed" \
  || fail "$CONTAINERS Docker containers still running"

# Verify endpoint API returns 0 after service deletion (service gone → 404 or empty)
EP_FINAL=$(curl -sf "$TINYKUBE_API/apis/v1/namespaces/default/services/whoami/endpoints" \
  2>/dev/null | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "gone")
[ "$EP_FINAL" = "gone" ] || [ "${EP_FINAL:-0}" -eq 0 ] \
  && pass "endpoint API returns 0 or 404 after service deletion" \
  || fail "endpoint API still returns endpoints after deletion: $EP_FINAL"

# ── Summary ────────────────────────────────────────────────────────────────────
echo
echo "════════════════════════════════"
echo "  Results: $PASS passed, $FAIL failed"
echo "════════════════════════════════"

[ "$FAIL" -eq 0 ] && exit 0 || exit 1
