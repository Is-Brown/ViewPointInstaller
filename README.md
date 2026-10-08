# Project Viewpoint Setup Assistant

Windows x64 helper for installing Project Viewpoint for Project Zomboid Build 42.

## Download
Grab `ProjectViewpointInstaller.exe` from this repository and run it. No install step is needed.

## What it does
1. **Open required Workshop items**: ZombieBuddy, Project Viewpoint, and the 6244 3D models pack. You subscribe in Steam yourself.
2. **Install / Update ZombieBuddy**: checks that the ZombieBuddy Workshop files have downloaded, then opens the official ZombieBuddy Windows installer release.
3. **Verify installation**: a quick check of the game files and Workshop downloads.
4. **FINAL CHECK**: a full readiness test covering `ZombieBuddy.jar`, `zbNative.dll`, the Workshop content and the launcher configuration. It reports PASS / WARN / FAIL and only says "ready" when every required check passes.

**Optional: Install ZombieBuddy Extensions.** Installs the unofficial [B42] ZombieBuddy Extensions add-on, which adds cached author verification, ZombieBuddy in locally hosted Coop servers, and startup-stability fixes. It follows the add-on author's manual steps:
- It refuses to run while Project Zomboid or a server is open.
- It backs up your current `ZombieBuddy.jar` and `zbNative.dll` to `ViewpointInstallerBackups\` in the game folder, with a timestamp.
- It installs the Extensions `ZombieBuddy.jar` and copies the current `zbNative.dll` from original ZombieBuddy.

Extensions is **not required**. The old B42.21 ZombieBuddy patch step has been removed because official ZombieBuddy no longer needs it.

## Safety
The program does not silently download or run any third-party EXE. ZombieBuddy is a Java agent with full system permissions, so you approve the official installer yourself. The official ZombieBuddy documentation warns users to install Java mods only from sources they trust.

## Upstream references
- Project Viewpoint Workshop ID: 3809306528
- ZombieBuddy Workshop ID: 3619862853
- 6244 3D Models Workshop ID: 3810302175
- [B42] ZombieBuddy Extensions Workshop ID (optional): 3807686870
- Official ZombieBuddy Windows installer release: https://github.com/zed-0xff/ZombieBuddy/releases/tag/windows_installer_4.2

## Build
Requires Go 1.23+. The included `rsrc_windows_amd64.syso` embeds the manifest that enables modern Windows visual styles and DPI awareness.
```text
go build -ldflags="-H=windowsgui -s -w" -o ProjectViewpointInstaller.exe .
```
To regenerate the manifest resource after editing `ProjectViewpointInstaller.exe.manifest`:
```text
go install github.com/akavel/rsrc@latest
rsrc -manifest ProjectViewpointInstaller.exe.manifest -o rsrc_windows_amd64.syso
```
