#!/usr/bin/env bash
# Local dev loop for the Mini App phone test: brings up Postgres, opens a
# cloudflared quick tunnel, captures its ephemeral HTTPS URL, and runs the app
# with PUBLIC_URL pointing at it. The bot's "Open App" button reads PUBLIC_URL
# at runtime, so no BotFather edits are needed when the tunnel URL changes.
#
# Prereqs: docker, cloudflared, a built frontend (make web-build), and a .env
# with BOT_TOKEN / ALLOWED_USER_IDS / DATABASE_URL.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> Starting Postgres"
docker compose up -d postgres >/dev/null

# Always rebuild: a stale dist on a phone test is a debugging trap, and the
# Vite build of this shell takes seconds.
echo "==> Building Mini App (web/dist)"
(cd web && npm run build)

echo "==> Opening cloudflared quick tunnel"
TUNNEL_LOG="$(mktemp)"
# Force an empty config: cloudflared merges ~/.cloudflared/config.yml even for a
# quick tunnel, and its `ingress` rules outrank --url. A config left there by an
# unrelated project answers our tunnel hostname with its own catch-all (typically
# `http_status:404`), so the app is up and reachable locally yet 404s through the
# tunnel — an expensive thing to debug. An empty config keeps --url authoritative.
TUNNEL_CFG_DIR="$(mktemp -d)"
TUNNEL_CFG="$TUNNEL_CFG_DIR/empty.yml"
: >"$TUNNEL_CFG"
cloudflared --config "$TUNNEL_CFG" tunnel --url http://localhost:8080 >"$TUNNEL_LOG" 2>&1 &
CF_PID=$!
trap 'kill "$CF_PID" 2>/dev/null || true; rm -rf "$TUNNEL_CFG_DIR"' EXIT

PUBLIC_URL=""
for _ in $(seq 1 30); do
  PUBLIC_URL="$(grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' "$TUNNEL_LOG" | head -1 || true)"
  [ -n "$PUBLIC_URL" ] && break
  sleep 1
done

if [ -z "$PUBLIC_URL" ]; then
  echo "!! Could not obtain a tunnel URL. cloudflared output:" >&2
  cat "$TUNNEL_LOG" >&2
  exit 1
fi

echo "==> Tunnel URL: $PUBLIC_URL"
echo "==> Open the bot in Telegram and tap /start → Open App"
PUBLIC_URL="$PUBLIC_URL" go run ./cmd/warden run
