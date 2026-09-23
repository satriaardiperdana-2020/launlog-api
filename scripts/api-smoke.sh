#!/usr/bin/env bash
set -euo pipefail

: "${APP_BINARY:?APP_BINARY must point to a freshly built API binary}"
: "${E2E_DATABASE_URL:?E2E_DATABASE_URL must point to a disposable isolated database}"

port="${E2E_HTTP_PORT:-18080}"
tmp="$(mktemp -d)"
log_file="$tmp/api.log"
response_file="$tmp/response"
pid=""

cleanup() {
	if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
		kill -TERM "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	rm -rf "$tmp"
}
trap cleanup EXIT

APP_ENV=integration \
HTTP_HOST=127.0.0.1 \
HTTP_PORT="$port" \
DATABASE_URL="$E2E_DATABASE_URL" \
JWT_SIGNING_SECRET=issue-016-isolated-smoke-signing-key-only \
JWT_ACCESS_TOKEN_TTL=1m \
JWT_REFRESH_TOKEN_TTL=1h \
SHUTDOWN_TIMEOUT=5s \
"$APP_BINARY" >"$log_file" 2>&1 &
pid=$!

base_url="http://127.0.0.1:$port"
ready=false
for _ in $(seq 1 100); do
	if curl --silent --show-error --max-time 2 "$base_url/readyz" -o "$response_file" 2>/dev/null; then
		ready=true
		break
	fi
	if ! kill -0 "$pid" 2>/dev/null; then
		break
	fi
	sleep 0.1
done
if [[ "$ready" != true ]]; then
	cat "$log_file" >&2
	echo 'API did not become ready against the isolated PostgreSQL database' >&2
	exit 1
fi

status="$(curl --silent --show-error --max-time 2 -o "$response_file" -w '%{http_code}' "$base_url/livez")"
[[ "$status" == 200 ]] || { echo "GET /livez returned $status" >&2; exit 1; }
grep -q '"status":"ok"' "$response_file"

status="$(curl --silent --show-error --max-time 2 -o "$response_file" -w '%{http_code}' "$base_url/readyz")"
[[ "$status" == 200 ]] || { echo "GET /readyz returned $status" >&2; exit 1; }

status="$(curl --silent --show-error --max-time 2 -D "$tmp/headers" -o "$response_file" -w '%{http_code}' "$base_url/health")"
[[ "$status" == 200 ]] || { echo "GET /health returned $status" >&2; exit 1; }
grep -qi '^X-Content-Type-Options: nosniff' "$tmp/headers"

status="$(curl --silent --show-error --max-time 2 -o "$response_file" -w '%{http_code}' "$base_url/auth/me")"
[[ "$status" == 401 ]] || { echo "unauthenticated GET /auth/me returned $status, want 401" >&2; exit 1; }

status="$(curl --silent --show-error --max-time 2 -o "$response_file" -w '%{http_code}' \
	-H 'Content-Type: application/json' -d '{"email":"missing-user@example.test","password":"invalid-password"}' \
	"$base_url/auth/login")"
[[ "$status" == 401 ]] || { echo "invalid POST /auth/login returned $status, want 401" >&2; exit 1; }

kill -TERM "$pid"
for _ in $(seq 1 60); do
	if ! kill -0 "$pid" 2>/dev/null; then
		break
	fi
	sleep 0.1
done
if kill -0 "$pid" 2>/dev/null; then
	cat "$log_file" >&2
	echo 'API did not shut down within its configured timeout after SIGTERM' >&2
	exit 1
fi
wait "$pid"
pid=""

echo 'API process smoke test passed: health, readiness, auth denial, security headers, and SIGTERM shutdown.'
