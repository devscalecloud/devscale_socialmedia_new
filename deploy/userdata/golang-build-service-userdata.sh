#!/bin/bash
set -euxo pipefail

# Log everything for debugging
exec > >(tee /var/log/user-data.log | logger -t user-data -s 2>/dev/console) 2>&1

# -----------------------------
# CONFIG
# -----------------------------
REPO_URL="https://github.com/devscalecloud/devscale_socialmedia_new.git"
APP_DIR="/opt/devscale_social_media_app"
APP_BIN="devscale_social_media_app"
APP_USER="devscale"
GO_VERSION="1.21.5"

# -----------------------------
# Update and install dependencies
# -----------------------------
dnf -y update
dnf -y install git   # ❌ DO NOT install curl (avoid conflict)

# -----------------------------
# Install Go (stable version)
# -----------------------------
cd /tmp
curl -LO https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz

rm -rf /usr/local/go
tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz

# Set PATH
export PATH=$PATH:/usr/local/go/bin
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile

# -----------------------------
# Create app user
# -----------------------------
id -u "$APP_USER" >/dev/null 2>&1 || \
useradd --system --create-home --shell /usr/sbin/nologin "$APP_USER"

# -----------------------------
# Clone repository
# -----------------------------
rm -rf "$APP_DIR"
git clone "$REPO_URL" "$APP_DIR"

cd "$APP_DIR"

# -----------------------------
# Fix Go build cache issue
# -----------------------------
export HOME=/root
export GOCACHE=/root/.cache/go-build
mkdir -p "$GOCACHE"

# -----------------------------
# Build application
# -----------------------------
/usr/local/go/bin/go build -o "$APP_BIN" .

# Set permissions
chown -R "$APP_USER:$APP_USER" "$APP_DIR"

# -----------------------------
# Create systemd service
# -----------------------------
cat >/etc/systemd/system/devscale-socialmedia.service <<EOF
[Unit]
Description=DevScale Social Media Go App
After=network.target

[Service]
Type=simple
User=$APP_USER
Group=$APP_USER
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/$APP_BIN
Environment=PORT=8080
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# -----------------------------
# Start service
# -----------------------------
systemctl daemon-reexec
systemctl daemon-reload
systemctl enable devscale-socialmedia.service
systemctl start devscale-socialmedia.service

# -----------------------------
# Verify service
# -----------------------------
systemctl is-active --quiet devscale-socialmedia.service && echo "✅ Service is running"

echo "✅ USER DATA COMPLETED SUCCESSFULLY"
echo "App should be running on port 8080"