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

# Locate the attractor binary: prefer one next to this script, else fall back to PATH.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ATTRACTOR="$SCRIPT_DIR/../attractor"
[ -x "$ATTRACTOR" ] || ATTRACTOR="attractor"

ANTHROPIC_API_KEY="$(load_secret ANTHROPIC_API_KEY)" \
OPENAI_API_KEY="$(load_secret OPENAI_API_KEY)" \
exec "$ATTRACTOR" "$@"
