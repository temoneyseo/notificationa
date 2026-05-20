#!/bin/sh
set -eu

ENV_FILE=/usr/local/etc/notification-hub/notification-hub.env
if [ -f "$ENV_FILE" ]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

exec /usr/local/bin/notification-hub --log /usr/local/var/log/notification-hub.log
