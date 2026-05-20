#!/bin/sh
set -eu

PLIST_NAME=com.notification-hub.plist
PLIST_TARGET=/Library/LaunchDaemons/$PLIST_NAME

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root: sudo sh deploy/launchd/install.sh" >&2
  exit 1
fi

install -d -m 0755 /usr/local/etc/notification-hub
install -d -m 0755 /usr/local/var/notification-hub
install -d -m 0755 /usr/local/var/log

if [ ! -f /usr/local/etc/notification-hub/notification-hub.env ]; then
  install -m 0600 deploy/launchd/notification-hub.env.example /usr/local/etc/notification-hub/notification-hub.env
  echo "Created /usr/local/etc/notification-hub/notification-hub.env. Edit it before starting the service."
fi

install -m 0755 deploy/launchd/notification-hub-wrapper.sh /usr/local/bin/notification-hub-launchd
install -m 0644 deploy/launchd/$PLIST_NAME "$PLIST_TARGET"
chown root:wheel "$PLIST_TARGET"
launchctl bootstrap system "$PLIST_TARGET" 2>/dev/null || true
launchctl enable system/com.notification-hub

echo "Installed com.notification-hub. Edit /usr/local/etc/notification-hub/notification-hub.env, then run:"
echo "  sudo launchctl kickstart -k system/com.notification-hub"
echo "  sudo launchctl print system/com.notification-hub"
