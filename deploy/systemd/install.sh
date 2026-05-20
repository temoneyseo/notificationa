#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root: sudo sh deploy/systemd/install.sh" >&2
  exit 1
fi

if ! id notification-hub >/dev/null 2>&1; then
  useradd --system --home /var/lib/notification-hub --shell /usr/sbin/nologin notification-hub
fi

install -d -o notification-hub -g notification-hub -m 0750 /var/lib/notification-hub
install -d -o notification-hub -g notification-hub -m 0750 /var/log/notification-hub
install -d -o root -g root -m 0755 /etc/notification-hub

if [ ! -f /etc/notification-hub/notification-hub.env ]; then
  install -o root -g root -m 0600 deploy/systemd/notification-hub.env.example /etc/notification-hub/notification-hub.env
  echo "Created /etc/notification-hub/notification-hub.env. Edit it before starting the service."
fi

install -o root -g root -m 0644 deploy/systemd/notification-hub.service /etc/systemd/system/notification-hub.service
systemctl daemon-reload
systemctl enable notification-hub.service

echo "Installed notification-hub.service. Edit /etc/notification-hub/notification-hub.env, then run:"
echo "  sudo systemctl start notification-hub"
echo "  sudo systemctl status notification-hub"
