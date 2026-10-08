# Project Viewpoint Setup Assistant

Windows x64 helper for installing Project Viewpoint for Project Zomboid Build 42.

## What it does
- Detects Steam and Project Zomboid across Steam libraries.
- Opens the required Workshop items for user-controlled subscription.
- Checks that Workshop content has actually downloaded.
- Opens the official ZombieBuddy Windows installer release.
- Applies the downloaded B42.21 replacement JAR with a timestamped backup.
- Runs a final readiness check for ZombieBuddy, `zbNative.dll`, Viewpoint, 3D models, the fix package, and launcher configuration.
- Reports PASS / WARN / FAIL results and only reports success when all required checks pass.

## Safety
The program does not silently download or execute an arbitrary third-party EXE. ZombieBuddy is a Java agent with full system permissions, so the official installer remains user-approved. The official ZombieBuddy documentation warns users to install Java mods only from sources they trust.

## Current upstream references
- Project Viewpoint Workshop ID: 3809306528
- ZombieBuddy Workshop ID: 3619862853
- B42.21 fix/extensions Workshop ID: 3807686870
- 6244 3D Models Workshop ID: 3810302175
- Official ZombieBuddy Windows installer release: https://github.com/zed-0xff/ZombieBuddy/releases/tag/windows_installer_4.2

## Build
```text
go build -ldflags="-H=windowsgui -s -w" -o ProjectViewpointInstaller.exe .
```
