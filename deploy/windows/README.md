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
