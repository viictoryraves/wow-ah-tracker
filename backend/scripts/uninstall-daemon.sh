#!/bin/sh
LABEL=com.wow-ah-tracker
launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null
rm -f "$HOME/Library/LaunchAgents/$LABEL.plist"
echo "uninstalled (collected data in data/wow.db is kept)"
