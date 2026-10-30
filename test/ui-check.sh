#!/bin/sh
# Runs inside a throwaway Alpine container with host networking (see make ui-check).
# The argument picks another browser script (default ui-check.mjs).
set -e
SCRIPT=${1:-ui-check.mjs}
apk add --no-cache chromium nodejs npm >/dev/null
mkdir -p /tmp/run /shots && cd /tmp/run
npm init -y >/dev/null && npm i puppeteer-core@24 --no-audit --no-fund >/dev/null 2>&1
cp "/test/$SCRIPT" . && node "$SCRIPT"
