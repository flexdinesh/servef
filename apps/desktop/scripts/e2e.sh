#!/bin/sh
# Real-app smoke (decision 0094): the app built with -tags e2e (mygo's
# automation plugin) on a display, driving its page against the real srcz
# binary (SRCZ_DESKTOP_BINARY). Run it under a display and a session bus, as
# the Dagger desktop group does:
#
#   xvfb-run -a dbus-run-session -- sh apps/desktop/scripts/e2e.sh
#
# Default: correctness. SRCZ_DESKTOP_EXTENDED=1 selects resource scenarios
# only, including RSS settling waits. SRCZ_DESKTOP_APP_BINARY may name the
# e2e-tagged app built from the same immutable snapshot; otherwise tests build it.
# Writes screenshots, report.json, app.log, and (extended only) RSS files to
# $SRCZ_DESKTOP_E2E_OUT (default .dev/desktop-e2e).
set -eu

SRCZ_DESKTOP_E2E_OUT="${SRCZ_DESKTOP_E2E_OUT:-$(pwd)/.dev/desktop-e2e}"
export SRCZ_DESKTOP_E2E_OUT
test_name=TestWebViewApp
if [ "${SRCZ_DESKTOP_EXTENDED:-}" = 1 ]; then
  test_name=TestWebViewResources
fi
GOWORK=off go -C apps/desktop test -tags e2e -count=1 -timeout 15m -run "^${test_name}$" -v ./e2e
