#!/bin/sh
# Builds the server and installs it as a launchd agent (macOS) that starts at
# login, restarts if it crashes, and keeps collecting price snapshots.
#
# launchd agents may not read ~/Documents (macOS privacy protection), so the
# runtime files (binary, .env, database, log) live in Application Support.
set -eu
SRC="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$HOME/Library/Application Support/wow-ah-tracker"
LABEL=com.wow-ah-tracker
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"

[ -f "$SRC/.env" ] || { echo "missing $SRC/.env (copy .env.example and fill it in)"; exit 1; }

mkdir -p "$DEST/data" "$HOME/Library/LaunchAgents"
(cd "$SRC" && go build -o "$DEST/wow-server" ./cmd/server)
cp "$SRC/scripts/run.sh" "$DEST/run.sh"
cp "$SRC/.env" "$DEST/.env"
chmod 700 "$DEST/run.sh" "$DEST/wow-server"
chmod 600 "$DEST/.env"
# Keep history collected so far, but never overwrite the daemon's own database.
if [ -f "$SRC/data/wow.db" ] && [ ! -f "$DEST/data/wow.db" ]; then
  cp "$SRC/data/wow.db" "$DEST/data/wow.db"
fi

cat > "$PLIST" <<PLISTEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key><array><string>$DEST/run.sh</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>30</integer>
  <key>StandardOutPath</key><string>$DEST/daemon.log</string>
  <key>StandardErrorPath</key><string>$DEST/daemon.log</string>
</dict></plist>
PLISTEOF

# Unload the old service and wait until it is really gone: loading again straight after
# unloading can fail with "Input/output error" and leave nothing running.
launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
i=0
while launchctl print "gui/$(id -u)/$LABEL" >/dev/null 2>&1 && [ "$i" -lt 20 ]; do
  sleep 0.5
  i=$((i + 1))
done

# Load the new one, retrying a few times in case launchd is still settling.
n=0
until launchctl bootstrap "gui/$(id -u)" "$PLIST" 2>/dev/null; do
  n=$((n + 1))
  if [ "$n" -ge 6 ]; then
    echo "could not start the daemon: launchctl bootstrap kept failing" >&2
    exit 1
  fi
  sleep 1
done
echo "installed in: $DEST"
echo "logs:         $DEST/daemon.log"
