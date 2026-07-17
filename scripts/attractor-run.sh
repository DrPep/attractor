#!/usr/bin/env bash
# attractor-run.sh — run Attractor with provider keys pulled from the macOS Keychain.
#
# One-time setup (stores each key as a generic password in your login keychain):
#
#   security add-generic-password -a "$USER" -s ANTHROPIC_API_KEY -w   # prompts for the secret
#   security add-generic-password -a "$USER" -s OPENAI_API_KEY    -w
#
# Usage (args are passed straight through to attractor):
#
#   ./scripts/attractor-run.sh run examples/hello.dot
#   ./scripts/attractor-run.sh chat
#
# The keys are injected only into the attractor process — they are never
# exported into your shell. The first run may pop a Keychain auth prompt;
# click "Always Allow" to whitelist the `security` binary.

set -euo pipefail

# load_secret <service> — print a secret from the login keychain, empty if absent.
load_secret() {
  security find-generic-password -s "$1" -w 2>/dev/null || true
}

# Rebuild from the working tree so a run never silently executes a stale binary.
# Go's build cache makes this a no-op when nothing changed. If the source tree or
# the go toolchain isn't there, fall back to a prebuilt binary or one on PATH.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ATTRACTOR="$REPO_ROOT/attractor"

if [ -f "$REPO_ROOT/go.mod" ] && command -v go >/dev/null 2>&1; then
  go build -C "$REPO_ROOT" -o "$ATTRACTOR" ./cmd/attractor
fi
[ -x "$ATTRACTOR" ] || ATTRACTOR="attractor"

ANTHROPIC_API_KEY="$(load_secret ANTHROPIC_API_KEY)" \
OPENAI_API_KEY="$(load_secret OPENAI_API_KEY)" \
exec "$ATTRACTOR" "$@"
