#!/bin/sh
# Real-app smoke (decision 0094): the app built with -tags e2e (mygo's
# automation plugin) on a display, driving its page against the real servef
# binary (SERVEF_DESKTOP_BINARY). Run it under a display and a session bus, as
# the Dagger desktop group does:
#
#   xvfb-run -a dbus-run-session -- sh apps/desktop/scripts/e2e.sh
#
# Writes screenshots, rss.txt, report.json, and app.log to
# $SERVEF_DESKTOP_E2E_OUT (default .dev/desktop-e2e).
set -eu

SERVEF_DESKTOP_E2E_OUT="${SERVEF_DESKTOP_E2E_OUT:-$(pwd)/.dev/desktop-e2e}"
export SERVEF_DESKTOP_E2E_OUT
GOWORK=off go -C apps/desktop test -tags e2e -count=1 -timeout 15m -v ./e2e
