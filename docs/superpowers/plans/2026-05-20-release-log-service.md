# Release Log Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish cross-platform release binaries, add `--log` file output, and provide service deployment templates for Linux, macOS, and Windows.

**Architecture:** Keep the application as a single Go binary. Add a tiny CLI setup layer in `cmd/notification-hub/main.go` that configures logging before the service is initialized. Add release automation and service deployment assets outside runtime code so operations support does not complicate the service internals.

**Tech Stack:** Go 1.26, standard library `flag`/`log`/`io`/`os`, GitHub Actions, systemd, launchd, WinSW.

---

## File Structure

- Modify `cmd/notification-hub/main.go`: parse `--log`, configure `log` output, pass the selected writer to inbound message logging, and close the log file on shutdown.
- Create `cmd/notification-hub/main_test.go`: unit tests for log setup without starting the server.
- Create `.github/workflows/release.yml`: tag-triggered/manual release workflow that tests and builds release archives.
- Create `deploy/systemd/notification-hub.service`: Linux service unit.
- Create `deploy/systemd/notification-hub.env.example`: Linux environment template.
- Create `deploy/systemd/install.sh`: Linux install helper for directories, env file, unit install, and daemon reload.
- Create `deploy/launchd/com.notification-hub.plist`: macOS launchd plist.
- Create `deploy/launchd/notification-hub.env.example`: macOS environment template.
- Create `deploy/launchd/notification-hub-wrapper.sh`: macOS wrapper that loads env values and runs the binary with `--log`.
- Create `deploy/launchd/install.sh`: macOS install helper for directories, wrapper, env file, plist install, and bootstrap.
- Create `deploy/windows/notification-hub.winsw.xml`: Windows service wrapper config.
- Create `deploy/windows/README.md`: Windows service installation instructions.
- Modify `README.md`: document releases, `--log`, and platform service deployment.

---

### Task 1: Add CLI Log Setup Tests

**Files:**
- Create: `cmd/notification-hub/main_test.go`
- Modify: `cmd/notification-hub/main.go`

- [ ] **Step 1: Add test file with failing tests**

Create `cmd/notification-hub/main_test.go`:

```go
package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureLogOutputWithEmptyPathKeepsDefaultWriter(t *testing.T) {
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)

	writer, cleanup, err := configureLogOutput("")
	if err != nil {
		t.Fatalf("configureLogOutput returned error: %v", err)
	}
	defer cleanup()

	if writer != &buf {
		t.Fatalf("writer = %T, want current log writer", writer)
	}
	log.Print("default log target")
	if !strings.Contains(buf.String(), "default log target") {
		t.Fatalf("default log output = %q, want message", buf.String())
	}
}

func TestConfigureLogOutputWithPathAppendsToFile(t *testing.T) {
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	logPath := filepath.Join(t.TempDir(), "notification-hub.log")
	if err := os.WriteFile(logPath, []byte("existing\n"), 0644); err != nil {
		t.Fatalf("write existing log: %v", err)
	}

	writer, cleanup, err := configureLogOutput(logPath)
	if err != nil {
		t.Fatalf("configureLogOutput returned error: %v", err)
	}
	defer cleanup()

	log.Print("new log line")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(content), "existing\n") {
		t.Fatalf("log file content = %q, want existing content preserved", string(content))
	}
	if !strings.Contains(string(content), "new log line") {
		t.Fatalf("log file content = %q, want new log line", string(content))
	}
	if writer == nil {
		t.Fatal("writer is nil")
	}
}

func TestConfigureLogOutputReturnsErrorForMissingDirectory(t *testing.T) {
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	missingPath := filepath.Join(t.TempDir(), "missing", "notification-hub.log")

	_, cleanup, err := configureLogOutput(missingPath)
	if err == nil {
		cleanup()
		t.Fatal("configureLogOutput returned nil error, want failure for missing directory")
	}
	if !strings.Contains(err.Error(), "open log file") {
		t.Fatalf("error = %v, want open log file context", err)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./cmd/notification-hub
```

Expected: FAIL because `configureLogOutput` is undefined.

- [ ] **Step 3: Add minimal log setup implementation**

Modify `cmd/notification-hub/main.go` imports to include `flag`, `fmt`, and `io`:

```go
import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/notification-hub/internal/adapters/discord"
	"github.com/user/notification-hub/internal/adapters/telegram"
	"github.com/user/notification-hub/internal/ai"
	"github.com/user/notification-hub/internal/config"
	"github.com/user/notification-hub/internal/http/handlers"
	"github.com/user/notification-hub/internal/security"
	"github.com/user/notification-hub/internal/server"
	"github.com/user/notification-hub/internal/service"
	"github.com/user/notification-hub/internal/storage/sqlite"
)
```

Replace the start of `run` with:

```go
func run() error {
	logPath := flag.String("log", "", "append logs to the specified file")
	flag.Parse()

	logWriter, cleanupLog, err := configureLogOutput(*logPath)
	if err != nil {
		return err
	}
	defer cleanupLog()

	cfg, err := config.Load()
```

Change inbound setup in `run` from:

```go
		InboundMessageWriter: os.Stdout,
```

to:

```go
		InboundMessageWriter: logWriter,
```

Add this helper below `run` and above `startInboundListeners`:

```go
func configureLogOutput(path string) (io.Writer, func(), error) {
	if path == "" {
		return log.Writer(), func() {}, nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open log file %q: %w", path, err)
	}
	log.SetOutput(file)
	return file, func() {
		if err := file.Close(); err != nil {
			log.Printf("close log file: %v", err)
		}
	}, nil
}
```

- [ ] **Step 4: Run focused tests**

Run:

```bash
go test ./cmd/notification-hub
```

Expected: PASS.

- [ ] **Step 5: Run all tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit log support**

Run:

```bash
git add cmd/notification-hub/main.go cmd/notification-hub/main_test.go
git commit -m "feat: add log file flag"
```

Expected: commit succeeds. If hooks fail, fix the underlying issue and create a new commit.

---

### Task 2: Add Release Workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create workflow directory**

Run:

```bash
mkdir -p .github/workflows
```

Expected: `.github/workflows` exists.

- [ ] **Step 2: Add release workflow**

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:

permissions:
  contents: write

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Install SQLite build dependencies
        run: sudo apt-get update && sudo apt-get install -y gcc libc6-dev libsqlite3-dev

      - name: Test
        run: go test ./...

  build:
    name: Build ${{ matrix.goos }}-${{ matrix.goarch }}
    needs: test
    strategy:
      fail-fast: false
      matrix:
        include:
          - goos: linux
            goarch: amd64
            runner: ubuntu-latest
            archive: tar.gz
          - goos: linux
            goarch: arm64
            runner: ubuntu-24.04-arm
            archive: tar.gz
          - goos: darwin
            goarch: amd64
            runner: macos-13
            archive: tar.gz
          - goos: darwin
            goarch: arm64
            runner: macos-14
            archive: tar.gz
          - goos: windows
            goarch: amd64
            runner: windows-latest
            archive: zip
    runs-on: ${{ matrix.runner }}
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Build binary
        shell: bash
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          CGO_ENABLED: '1'
        run: |
          set -euo pipefail
          mkdir -p dist
          version="${GITHUB_REF_NAME:-dev}"
          binary="notification-hub"
          if [ "${GOOS}" = "windows" ]; then
            binary="notification-hub.exe"
          fi
          go build -trimpath -ldflags "-s -w -X main.version=${version}" -o "dist/${binary}" ./cmd/notification-hub
          cp README.md config.yaml.example .env.example dist/

      - name: Package Unix archive
        if: matrix.archive == 'tar.gz'
        shell: bash
        run: |
          set -euo pipefail
          archive="notification-hub-${GITHUB_REF_NAME}-${{ matrix.goos }}-${{ matrix.goarch }}.tar.gz"
          tar -C dist -czf "${archive}" .
          mkdir -p artifacts
          mv "${archive}" artifacts/

      - name: Package Windows archive
        if: matrix.archive == 'zip'
        shell: pwsh
        run: |
          $archive = "notification-hub-${env:GITHUB_REF_NAME}-${{ matrix.goos }}-${{ matrix.goarch }}.zip"
          Compress-Archive -Path dist\* -DestinationPath $archive
          New-Item -ItemType Directory -Force -Path artifacts | Out-Null
          Move-Item $archive artifacts\

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: notification-hub-${{ matrix.goos }}-${{ matrix.goarch }}
          path: artifacts/*
          if-no-files-found: error

  release:
    name: Publish GitHub Release
    needs: build
    runs-on: ubuntu-latest
    steps:
      - name: Download artifacts
        uses: actions/download-artifact@v4
        with:
          path: release-artifacts
          merge-multiple: true

      - name: Generate checksums
        run: |
          cd release-artifacts
          sha256sum * > checksums.txt

      - name: Publish release
        uses: softprops/action-gh-release@v2
        with:
          files: release-artifacts/*
          generate_release_notes: true
```

- [ ] **Step 3: Inspect workflow for syntax-sensitive indentation**

Run:

```bash
git diff -- .github/workflows/release.yml
```

Expected: workflow contains `test`, `build`, and `release` jobs, and all `run` blocks are indented under steps.

- [ ] **Step 4: Commit workflow**

Run:

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release builds"
```

Expected: commit succeeds. If hooks fail, fix the underlying issue and create a new commit.

---

### Task 3: Add Linux systemd Deployment Assets

**Files:**
- Create: `deploy/systemd/notification-hub.service`
- Create: `deploy/systemd/notification-hub.env.example`
- Create: `deploy/systemd/install.sh`

- [ ] **Step 1: Create systemd directory**

Run:

```bash
mkdir -p deploy/systemd
```

Expected: `deploy/systemd` exists.

- [ ] **Step 2: Add systemd unit**

Create `deploy/systemd/notification-hub.service`:

```ini
[Unit]
Description=Notification Hub
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=notification-hub
Group=notification-hub
EnvironmentFile=/etc/notification-hub/notification-hub.env
ExecStart=/usr/local/bin/notification-hub --log /var/log/notification-hub/notification-hub.log
Restart=on-failure
RestartSec=5s
WorkingDirectory=/var/lib/notification-hub
StateDirectory=notification-hub
LogsDirectory=notification-hub
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=/var/lib/notification-hub /var/log/notification-hub

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 3: Add Linux env template**

Create `deploy/systemd/notification-hub.env.example`:

```env
HTTP_ADDR=:8080
DATABASE_PATH=/var/lib/notification-hub/notification-hub.db
ENCRYPTION_KEY=change-me-32-bytes-minimum-secret
OPENAI_API_KEY=
OPENAI_MODEL=gpt-4o-mini
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_TIMEOUT=30s
ACP_ENDPOINT_URL=
ACP_AUTH_TOKEN=
ACP_ENABLED=false
ACP_DEFAULT_PROJECT=notification
ACP_DEFAULT_AGENT=triage
ACP_MIN_CONFIDENCE=0.8
ACP_ALLOWED_INTENTS=docs_request,incident,support_request
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
DISCORD_BOT_TOKEN=
DISCORD_CHANNEL_ID=
```

- [ ] **Step 4: Add Linux install helper**

Create `deploy/systemd/install.sh`:

```sh
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
```

- [ ] **Step 5: Make install helper executable**

Run:

```bash
chmod +x deploy/systemd/install.sh
```

Expected: file mode includes executable bit.

- [ ] **Step 6: Commit Linux service assets**

Run:

```bash
git add deploy/systemd/notification-hub.service deploy/systemd/notification-hub.env.example deploy/systemd/install.sh
git commit -m "feat: add systemd deployment assets"
```

Expected: commit succeeds. If hooks fail, fix the underlying issue and create a new commit.

---

### Task 4: Add macOS launchd Deployment Assets

**Files:**
- Create: `deploy/launchd/com.notification-hub.plist`
- Create: `deploy/launchd/notification-hub.env.example`
- Create: `deploy/launchd/notification-hub-wrapper.sh`
- Create: `deploy/launchd/install.sh`

- [ ] **Step 1: Create launchd directory**

Run:

```bash
mkdir -p deploy/launchd
```

Expected: `deploy/launchd` exists.

- [ ] **Step 2: Add launchd plist**

Create `deploy/launchd/com.notification-hub.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.notification-hub</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/notification-hub-launchd</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>/usr/local/var/notification-hub</string>
  <key>StandardOutPath</key>
  <string>/usr/local/var/log/notification-hub-launchd.out.log</string>
  <key>StandardErrorPath</key>
  <string>/usr/local/var/log/notification-hub-launchd.err.log</string>
</dict>
</plist>
```

- [ ] **Step 3: Add macOS env template**

Create `deploy/launchd/notification-hub.env.example`:

```env
HTTP_ADDR=:8080
DATABASE_PATH=/usr/local/var/notification-hub/notification-hub.db
ENCRYPTION_KEY=change-me-32-bytes-minimum-secret
OPENAI_API_KEY=
OPENAI_MODEL=gpt-4o-mini
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_TIMEOUT=30s
ACP_ENDPOINT_URL=
ACP_AUTH_TOKEN=
ACP_ENABLED=false
ACP_DEFAULT_PROJECT=notification
ACP_DEFAULT_AGENT=triage
ACP_MIN_CONFIDENCE=0.8
ACP_ALLOWED_INTENTS=docs_request,incident,support_request
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
DISCORD_BOT_TOKEN=
DISCORD_CHANNEL_ID=
```

- [ ] **Step 4: Add macOS wrapper script**

Create `deploy/launchd/notification-hub-wrapper.sh`:

```sh
#!/bin/sh
set -eu

ENV_FILE=/usr/local/etc/notification-hub/notification-hub.env
if [ -f "$ENV_FILE" ]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

exec /usr/local/bin/notification-hub --log /usr/local/var/log/notification-hub.log
```

- [ ] **Step 5: Add macOS install helper**

Create `deploy/launchd/install.sh`:

```sh
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
```

- [ ] **Step 6: Make macOS scripts executable**

Run:

```bash
chmod +x deploy/launchd/install.sh deploy/launchd/notification-hub-wrapper.sh
```

Expected: both files have executable bits.

- [ ] **Step 7: Commit macOS service assets**

Run:

```bash
git add deploy/launchd/com.notification-hub.plist deploy/launchd/notification-hub.env.example deploy/launchd/notification-hub-wrapper.sh deploy/launchd/install.sh
git commit -m "feat: add launchd deployment assets"
```

Expected: commit succeeds. If hooks fail, fix the underlying issue and create a new commit.

---

### Task 5: Add Windows WinSW Deployment Assets

**Files:**
- Create: `deploy/windows/notification-hub.winsw.xml`
- Create: `deploy/windows/README.md`

- [ ] **Step 1: Create Windows deploy directory**

Run:

```bash
mkdir -p deploy/windows
```

Expected: `deploy/windows` exists.

- [ ] **Step 2: Add WinSW XML**

Create `deploy/windows/notification-hub.winsw.xml`:

```xml
<service>
  <id>notification-hub</id>
  <name>Notification Hub</name>
  <description>Routes notifications to Telegram and Discord and collects inbound messages.</description>
  <executable>%BASE%\notification-hub.exe</executable>
  <arguments>--log %BASE%\logs\notification-hub.log</arguments>
  <workingdirectory>%BASE%</workingdirectory>
  <env name="HTTP_ADDR" value=":8080" />
  <env name="DATABASE_PATH" value="%BASE%\data\notification-hub.db" />
  <env name="ENCRYPTION_KEY" value="change-me-32-bytes-minimum-secret" />
  <env name="OPENAI_API_KEY" value="" />
  <env name="OPENAI_MODEL" value="gpt-4o-mini" />
  <env name="OPENAI_BASE_URL" value="https://api.openai.com/v1" />
  <env name="OPENAI_TIMEOUT" value="30s" />
  <env name="ACP_ENDPOINT_URL" value="" />
  <env name="ACP_AUTH_TOKEN" value="" />
  <env name="ACP_ENABLED" value="false" />
  <env name="ACP_DEFAULT_PROJECT" value="notification" />
  <env name="ACP_DEFAULT_AGENT" value="triage" />
  <env name="ACP_MIN_CONFIDENCE" value="0.8" />
  <env name="ACP_ALLOWED_INTENTS" value="docs_request,incident,support_request" />
  <env name="TELEGRAM_BOT_TOKEN" value="" />
  <env name="TELEGRAM_CHAT_ID" value="" />
  <env name="DISCORD_BOT_TOKEN" value="" />
  <env name="DISCORD_CHANNEL_ID" value="" />
  <log mode="roll-by-size">
    <sizeThreshold>10485760</sizeThreshold>
    <keepFiles>5</keepFiles>
  </log>
  <onfailure action="restart" delay="5 sec" />
</service>
```

- [ ] **Step 3: Add Windows install README**

Create `deploy/windows/README.md`:

```markdown
# Windows Service with WinSW

Notification Hub can run as a Windows service with [WinSW](https://github.com/winsw/winsw).

## Install

1. Download the Windows release archive for Notification Hub.
2. Extract it to `C:\notification-hub`.
3. Download the WinSW x64 executable and place it in the same directory as `notification-hub.exe`.
4. Rename the WinSW executable to `notification-hub-service.exe`.
5. Copy `deploy/windows/notification-hub.winsw.xml` to `C:\notification-hub\notification-hub-service.xml`.
6. Edit `notification-hub-service.xml` and set `ENCRYPTION_KEY`, bot tokens, OpenAI settings, ACP settings, and `HTTP_ADDR`.
7. Create data and log directories:

```powershell
New-Item -ItemType Directory -Force C:\notification-hub\data
New-Item -ItemType Directory -Force C:\notification-hub\logs
```

8. Install and start the service from an elevated PowerShell:

```powershell
cd C:\notification-hub
.\notification-hub-service.exe install
.\notification-hub-service.exe start
.\notification-hub-service.exe status
```

## Logs

Notification Hub writes application logs to `C:\notification-hub\logs\notification-hub.log` through `--log`. WinSW writes wrapper logs next to the service executable.

## Uninstall

```powershell
cd C:\notification-hub
.\notification-hub-service.exe stop
.\notification-hub-service.exe uninstall
```
```

- [ ] **Step 4: Commit Windows service assets**

Run:

```bash
git add deploy/windows/notification-hub.winsw.xml deploy/windows/README.md
git commit -m "feat: add windows service deployment assets"
```

Expected: commit succeeds. If hooks fail, fix the underlying issue and create a new commit.

---

### Task 6: Update README Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add release and log sections after Run Locally**

Modify `README.md` after the existing health check block:

```markdown
## Release Binaries

Tagged releases publish prebuilt archives for common platforms:

- macOS Apple Silicon: `notification-hub-<version>-darwin-arm64.tar.gz`
- macOS Intel: `notification-hub-<version>-darwin-amd64.tar.gz`
- Linux AMD64: `notification-hub-<version>-linux-amd64.tar.gz`
- Linux ARM64: `notification-hub-<version>-linux-arm64.tar.gz`
- Windows AMD64: `notification-hub-<version>-windows-amd64.zip`

Download the archive for your platform, extract it, copy `notification-hub` or `notification-hub.exe` onto your `PATH`, and configure the required environment variables.

## Log File Output

By default, Notification Hub writes process logs to the terminal. Use `--log` to append logs to a file:

```bash
ENCRYPTION_KEY=0123456789abcdef0123456789abcdef \
  notification-hub --log ./notification-hub.log
```

The log file must be in an existing directory. When `log_inbound_messages: true` is enabled in `config.yaml`, inbound Telegram and Discord message logs are written to the same target.
```

- [ ] **Step 2: Add service deployment section before Docker**

Modify `README.md` before `## Docker`:

```markdown
## Service Deployment

The `deploy/` directory contains templates for running Notification Hub as a host service. Service deployments need the same environment variables as local runs: `HTTP_ADDR`, `DATABASE_PATH`, `ENCRYPTION_KEY`, bot tokens, OpenAI settings, and optional ACP settings.

### Linux systemd

1. Copy the release binary to `/usr/local/bin/notification-hub`.
2. Install the service files:

```bash
sudo sh deploy/systemd/install.sh
```

3. Edit `/etc/notification-hub/notification-hub.env` and set real secrets.
4. Start the service:

```bash
sudo systemctl start notification-hub
sudo systemctl status notification-hub
```

The default database path is `/var/lib/notification-hub/notification-hub.db`. The default application log path is `/var/log/notification-hub/notification-hub.log`.

### macOS launchd

1. Copy the release binary to `/usr/local/bin/notification-hub`.
2. Install the launchd files:

```bash
sudo sh deploy/launchd/install.sh
```

3. Edit `/usr/local/etc/notification-hub/notification-hub.env` and set real secrets.
4. Restart the service:

```bash
sudo launchctl kickstart -k system/com.notification-hub
sudo launchctl print system/com.notification-hub
```

The default database path is `/usr/local/var/notification-hub/notification-hub.db`. The default application log path is `/usr/local/var/log/notification-hub.log`.

### Windows service

Use the WinSW example in `deploy/windows/`. The Windows service runs:

```powershell
notification-hub.exe --log logs\notification-hub.log
```

See `deploy/windows/README.md` for installation and uninstall commands.
```

- [ ] **Step 3: Run README grep checks**

Run:

```bash
grep -n "Release Binaries\|Log File Output\|Service Deployment\|systemd\|launchd\|WinSW" README.md
```

Expected: output includes all section names and platform names.

- [ ] **Step 4: Commit README updates**

Run:

```bash
git add README.md
git commit -m "docs: document releases and service deployment"
```

Expected: commit succeeds. If hooks fail, fix the underlying issue and create a new commit.

---

### Task 7: Final Verification and Cleanup

**Files:**
- Inspect: all changed files

- [ ] **Step 1: Run all tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Verify release workflow references expected platforms**

Run:

```bash
grep -n "darwin\|linux\|windows\|amd64\|arm64\|action-gh-release" .github/workflows/release.yml
```

Expected: output includes `darwin`, `linux`, `windows`, `amd64`, `arm64`, and `softprops/action-gh-release@v2`.

- [ ] **Step 3: Verify deployment paths are consistent**

Run:

```bash
grep -R "notification-hub.log\|notification-hub.env\|DATABASE_PATH\|ExecStart\|ProgramArguments" -n deploy README.md
```

Expected: output shows the same default paths documented in the design:

- Linux log: `/var/log/notification-hub/notification-hub.log`
- Linux env: `/etc/notification-hub/notification-hub.env`
- macOS log: `/usr/local/var/log/notification-hub.log`
- macOS env: `/usr/local/etc/notification-hub/notification-hub.env`
- Windows log: `logs\\notification-hub.log` or `logs\notification-hub.log`

- [ ] **Step 4: Review git status**

Run:

```bash
git status --short
```

Expected: only intentional files remain. Do not commit `.memsearch/`.

- [ ] **Step 5: Commit any remaining planned files**

If `git status --short` shows planned files that were not committed, add only those files and commit them:

```bash
git add docs/superpowers/specs/2026-05-20-release-log-service-design.md docs/superpowers/plans/2026-05-20-release-log-service.md
git commit -m "docs: add release service implementation plan"
```

Expected: commit succeeds if those files are still uncommitted. If there are no planned files left, skip this step.

- [ ] **Step 6: Prepare final summary**

Report:

- Tests run and result.
- Release platforms included.
- Service deployment templates added.
- Whether any files remain uncommitted.
- Whether `.memsearch/` was left untracked.

---

## Self-Review

Spec coverage:

- Release workflow: Task 2 covers `.github/workflows/release.yml`, tests, platform matrix, archives, checksums, GitHub Release upload.
- `--log`: Task 1 covers flag parsing, append-only file output, default behavior, inbound writer reuse, and tests.
- Service templates: Tasks 3, 4, and 5 cover systemd, launchd, and Windows WinSW files.
- Documentation: Task 6 covers release, log, and service docs.
- Verification: Task 7 covers all tests and consistency checks.

Placeholder scan: no `TBD`, `TODO`, `implement later`, or intentionally vague implementation steps remain.

Type consistency: `configureLogOutput(path string) (io.Writer, func(), error)` is used consistently by tests and implementation steps.
