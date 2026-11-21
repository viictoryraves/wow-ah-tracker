#!/bin/sh
# Entry point used by launchd: loads .env and runs the server (relative to this folder).
cd "$(dirname "$0")" || exit 1
set -a
. ./.env
set +a
exec ./wow-server
