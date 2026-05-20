# Release Builds, Log File Output, and Service Deployment Design

## Goal

Make Notification Hub easier to ship and operate as a standalone service. The project should publish cross-platform release binaries, allow operators to route process logs to a file with `--log`, and include platform service templates for Linux, macOS, and Windows.

## Scope

This change covers three areas:

1. GitHub Actions release builds for common desktop/server platforms.
2. A `--log` CLI flag for append-only log file output.
3. Service deployment templates and documentation.

It does not add an in-binary service installer, package manager publishing, or automatic host configuration.

## Release Builds

Add `.github/workflows/release.yml`.

The workflow runs on:

- `push` tags matching `v*`
- manual `workflow_dispatch`

The workflow should:

1. Check out the repository.
2. Set up Go.
3. Run `go test ./...`.
4. Build release archives for:
   - `darwin/arm64`
   - `darwin/amd64`
   - `linux/amd64`
   - `linux/arm64`
   - `windows/amd64`
5. Package Unix binaries as `.tar.gz` and Windows binaries as `.zip`.
6. Generate a checksum file.
7. Attach all artifacts to a GitHub Release.

The project uses SQLite. If the current SQLite driver requires CGO, the workflow should keep the build matrix practical rather than pretending all cross-compilation is equal. Native runners should be used where needed: macOS builds on macOS runners, Linux builds on Ubuntu runners, and Windows builds on Windows runners. Linux ARM64 can be attempted with Go's cross-compilation if dependencies allow it; if not, the workflow should be adjusted to the platforms that can be built reliably.

## CLI Log File Support

Add a `--log` flag to `notification-hub`:

```bash
notification-hub --log /var/log/notification-hub/notification-hub.log
```

Behavior:

- If `--log` is omitted, current logging behavior remains unchanged.
- If `--log` is provided, the process opens or creates the file in append mode.
- The log file uses `0644` permissions when created.
- Standard library `log` output is redirected to the file.
- Inbound message logging uses the same writer as the process log so service deployments have one predictable log target.
- If the file cannot be opened, startup fails with a clear error.

The flag should be parsed before configuration loading so startup errors and later logs share the configured destination when possible.

## Service Deployment Templates

Add service assets under `deploy/`:

```text
deploy/
  systemd/
    notification-hub.service
    notification-hub.env.example
    install.sh
  launchd/
    com.notification-hub.plist
    notification-hub.env.example
    notification-hub-wrapper.sh
    install.sh
  windows/
    notification-hub.winsw.xml
    README.md
```

### Linux systemd

Defaults:

- Binary: `/usr/local/bin/notification-hub`
- Environment file: `/etc/notification-hub/notification-hub.env`
- Database: `/var/lib/notification-hub/notification-hub.db`
- Log: `/var/log/notification-hub/notification-hub.log`
- HTTP address: `:8080`

The service should run `notification-hub --log /var/log/notification-hub/notification-hub.log` and load required environment from the env file.

### macOS launchd

Defaults:

- Binary: `/usr/local/bin/notification-hub`
- Wrapper: `/usr/local/bin/notification-hub-launchd`
- Env file: `/usr/local/etc/notification-hub/notification-hub.env`
- Database: `/usr/local/var/notification-hub/notification-hub.db`
- Log: `/usr/local/var/log/notification-hub.log`

Use a wrapper script so operators can manage environment variables in an env file instead of embedding secrets directly in the plist.

### Windows WinSW

Provide a WinSW XML example that runs `notification-hub.exe --log logs\\notification-hub.log`. The README should explain that users download WinSW, place the binary and XML together, edit environment values, and run WinSW install/start commands from an elevated shell.

## Documentation

Update `README.md` with:

- Downloading release binaries.
- Running with `--log`.
- Installing as a Linux systemd service.
- Installing as a macOS launchd service.
- Installing as a Windows service with WinSW.
- Required environment variables for service mode, especially `ENCRYPTION_KEY`, `DATABASE_PATH`, bot tokens, OpenAI settings, ACP settings, and `HTTP_ADDR`.

## Tests and Verification

Add tests around CLI setup where practical:

- `--log` writes standard logs to the requested file.
- Omitting `--log` preserves the default log writer path.

Run `go test ./...` before completion. Also inspect the release workflow and deployment files for path consistency.
