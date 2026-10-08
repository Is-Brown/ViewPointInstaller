//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const (
	appName          = "Project Viewpoint Setup Assistant"
	pzAppID          = "108600"
	workshopView     = "3809306528"
	workshopZB       = "3619862853"
	workshopExt      = "3807686870" // [B42] ZombieBuddy Extensions (optional add-on)
	workshopModels   = "3810302175"
	extModID         = "ZombieBuddy_Extensions"
	githubReleaseAPI = "https://api.github.com/repos/zed-0xff/ZombieBuddy/releases/tags/windows_installer_4.2"
	githubReleases   = "https://github.com/zed-0xff/ZombieBuddy/releases/tag/windows_installer_4.2"
)

var (
	appWindow *walk.MainWindow
	logView   *walk.TextEdit
	steamRoot string
	pzRoot    string
)

// ---------------------------------------------------------------------------
// UI helpers (thread-safe: may be called from worker goroutines)
// ---------------------------------------------------------------------------

// uiReady is closed once the main window has been created, so worker
// goroutines never touch appWindow before it exists.
var uiReady = make(chan struct{})

// onUI runs f on the UI thread via the main window's message loop.
func onUI(f func()) {
	<-uiReady
	appWindow.Synchronize(f)
}

func appendLog(s string) {
	line := time.Now().Format("15:04:05") + "  " + s + "\r\n"
	onUI(func() {
		if logView == nil {
			return
		}
		logView.AppendText(line)
	})
}

func showError(msg string) {
	onUI(func() {
		walk.MsgBox(appWindow, appName, msg, walk.MsgBoxIconError|walk.MsgBoxOK)
	})
}

func showInfo(msg string) {
	onUI(func() {
		walk.MsgBox(appWindow, appName, msg, walk.MsgBoxIconInformation|walk.MsgBoxOK)
	})
}

// hiddenCmd builds a command that does not flash a console window
// (the app is linked with -H=windowsgui, so cmd/tasklist would otherwise pop one up).
func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c
}

func openURL(url string) {
	if err := hiddenCmd("cmd", "/c", "start", "", url).Start(); err != nil {
		appendLog("ERROR opening URL: " + err.Error())
	}
}

// gameRunning reports whether Project Zomboid (or its dedicated server) is running,
// since ZombieBuddy.jar / zbNative.dll are locked while the game is open.
func gameRunning() bool {
	out, err := hiddenCmd("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	low := strings.ToLower(string(out))
	return strings.Contains(low, "\"projectzomboid64.exe\"") || strings.Contains(low, "\"projectzomboid32.exe\"") ||
		strings.Contains(low, "\"projectzomboidserver") || strings.Contains(low, "\"startserver64")
}

func openPZFolder() {
	if pzRoot == "" {
		detectSteamAndPZ()
	}
	if pzRoot == "" {
		showError("Project Zomboid installation was not found.")
		return
	}
	if err := exec.Command("explorer", pzRoot).Start(); err != nil {
		showError("Could not open the Project Zomboid folder: " + err.Error())
	}
}

// ---------------------------------------------------------------------------
// Main window
// ---------------------------------------------------------------------------

func main() {
	if runtime.GOOS != "windows" {
		return
	}
	detectSteamAndPZ()

	headerBg := walk.RGB(0x16, 0x1A, 0x20)
	btnFont := Font{Family: "Segoe UI", PointSize: 9}
	btnFontBold := Font{Family: "Segoe UI", PointSize: 9, Bold: true}
	btnMin := Size{Height: 36}

	go func() {
		time.Sleep(200 * time.Millisecond)
		appendLog("Steam: " + valueOrUnknown(steamRoot))
		appendLog("Project Zomboid: " + valueOrUnknown(pzRoot))
		appendLog("Click step 1 to open the Workshop pages. Steam subscription remains user-controlled.")
	}()

	err := MainWindow{
		AssignTo: &appWindow,
		Title:    appName,
		MinSize:  Size{Width: 760, Height: 580},
		Size:     Size{Width: 780, Height: 620},
		Font:     Font{Family: "Segoe UI", PointSize: 9},
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			// Dark header
			Composite{
				Background: SolidColorBrush{Color: headerBg},
				Layout:     VBox{Margins: Margins{Left: 20, Top: 16, Right: 20, Bottom: 16}, Spacing: 4},
				Children: []Widget{
					Label{
						Text:       "Project Viewpoint · Build 42 Setup Assistant",
						Font:       Font{Family: "Segoe UI", PointSize: 13, Bold: true},
						TextColor:  walk.RGB(0xFF, 0xFF, 0xFF),
						Background: SolidColorBrush{Color: headerBg},
					},
					Label{
						Text:       "Opens the required Workshop items, installs official ZombieBuddy, optionally adds ZombieBuddy Extensions, and verifies the result.",
						Font:       Font{Family: "Segoe UI", PointSize: 9},
						TextColor:  walk.RGB(0xA8, 0xB0, 0xBC),
						Background: SolidColorBrush{Color: headerBg},
					},
				},
			},
			// Body
			Composite{
				StretchFactor: 1,
				Layout:        VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 14}, Spacing: 10},
				Children: []Widget{
					GroupBox{
						Title:  "Installation Steps",
						Layout: Grid{Columns: 2, Spacing: 8, Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 10}},
						Children: []Widget{
							PushButton{Text: "1 · Open required Workshop items", Font: btnFont, MinSize: btnMin,
								OnClicked: func() { go openWorkshopPages() }},
							PushButton{Text: "2 · Install / Update ZombieBuddy", Font: btnFont, MinSize: btnMin,
								OnClicked: func() { go installZombieBuddy() }},
							PushButton{Text: "3 · Verify installation", Font: btnFont, MinSize: btnMin,
								OnClicked: func() { go verifyInstallation() }},
							PushButton{Text: "4 · FINAL CHECK — Ready to play", Font: btnFontBold, MinSize: btnMin,
								OnClicked: func() { go finalCheck() }},
							PushButton{Text: "Open Project Zomboid folder", Font: btnFont, MinSize: btnMin,
								OnClicked: openPZFolder},
						},
					},
					GroupBox{
						Title:  "ZombieBuddy Extensions (Optional)",
						Layout: Grid{Columns: 2, Spacing: 8, Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 10}},
						Children: []Widget{
							PushButton{
								Text:      "Swap JAR → Use Extensions Version",
								Font:      btnFont,
								MinSize:   btnMin,
								OnClicked: func() { go swapJarToExtensions() },
							},
							PushButton{
								Text:      "Restore JAR → Use Official ZombieBuddy",
								Font:      btnFont,
								MinSize:   btnMin,
								OnClicked: func() { go restoreJarToOfficial() },
							},
						},
					},
					GroupBox{
						Title:         "Activity Log",
						StretchFactor: 1,
						Layout:        VBox{Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 10}},
						Children: []Widget{
							TextEdit{
								AssignTo: &logView,
								ReadOnly: true,
								VScroll:  true,
								Font:     Font{Family: "Consolas", PointSize: 9},
							},
						},
					},
				},
			},
		},
	}.Create()
	if err != nil {
		walk.MsgBox(nil, appName, "Failed to start the UI: "+err.Error(), walk.MsgBoxIconError|walk.MsgBoxOK)
		return
	}
	close(uiReady)
	appWindow.Run()
}

// ---------------------------------------------------------------------------
// Installer logic (unchanged from the original, except dialog calls)
// ---------------------------------------------------------------------------

func valueOrUnknown(s string) string {
	if s == "" {
		return "NOT FOUND"
	}
	return s
}

func openWorkshopPages() {
	pages := []struct{ id, name string }{
		{workshopZB, "ZombieBuddy"},
		{workshopView, "Project Viewpoint"},
		{workshopModels, "6244 3D models for Viewpoint"},
	}
	for _, p := range pages {
		appendLog("Opening Workshop: " + p.name + " (" + p.id + ")")
		openURL("https://steamcommunity.com/sharedfiles/filedetails/?id=" + p.id)
		time.Sleep(300 * time.Millisecond)
	}
	appendLog("Subscribe to the items in Steam, then wait for Workshop downloads to finish.")
}

func installZombieBuddy() {
	if pzRoot == "" {
		appendLog("ERROR: Project Zomboid installation was not found.")
		return
	}
	appendLog("Checking ZombieBuddy Workshop files...")
	zbLib := findWorkshopFile(workshopZB, "ZombieBuddy.jar")
	if zbLib == "" {
		appendLog("ZombieBuddy is not downloaded yet. Click Workshop and subscribe first.")
		return
	}
	// The official installer is preferred because it patches the supported launch targets.
	appendLog("ZombieBuddy Workshop content found.")
	appendLog("Opening the official ZombieBuddy Windows installer release from GitHub...")
	openURL(githubReleases)
	appendLog("Download ZombieBuddyInstaller.exe from GitHub and run it; choose Install/Update and Both.")
}

// jarSwapPrecheck runs the checks common to both swap directions and returns
// (backupDir, stamp, ok). On any failure it logs and returns ok=false.
func jarSwapPrecheck(label string) (backupDir, stamp string, ok bool) {
	appendLog("--- " + label + " ---")
	if pzRoot == "" {
		appendLog("ERROR: Project Zomboid installation was not found.")
		return
	}
	if gameRunning() {
		appendLog("ERROR: Project Zomboid (or a server) is running. Close it and try again.")
		showError("Please close Project Zomboid and any Coop/dedicated server first.\n\nZombieBuddy files cannot be replaced while the game is running.")
		return
	}
	stamp = time.Now().Format("20060102-150405")
	backupDir = filepath.Join(pzRoot, "ViewpointInstallerBackups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		appendLog("ERROR creating backup folder: " + err.Error())
		return
	}
	ok = true
	return
}

// backupGameFile backs up a single file inside pzRoot, logging a message.
// Returns false on error, leaving the original untouched.
func backupGameFile(name, backupDir, stamp string) bool {
	cur := filepath.Join(pzRoot, name)
	if _, err := os.Stat(cur); err != nil {
		return true // file doesn't exist yet — nothing to back up
	}
	backup := filepath.Join(backupDir, name+"."+stamp+".bak")
	if err := copyFile(cur, backup); err != nil {
		appendLog("ERROR backing up " + name + ": " + err.Error() + " — nothing was changed.")
		return false
	}
	appendLog("Backed up " + name + " → " + backup)
	return true
}

// swapJarToExtensions replaces ZombieBuddy.jar with the Extensions version.
// Also refreshes zbNative.dll from the original ZombieBuddy workshop folder.
func swapJarToExtensions() {
	backupDir, stamp, ok := jarSwapPrecheck("Swap JAR → Extensions version")
	if !ok {
		return
	}

	// Extensions JAR
	if findWorkshopDir(workshopExt) == "" {
		appendLog("ZombieBuddy Extensions is not downloaded. Opening its Workshop page (ID " + workshopExt + ")...")
		appendLog("Subscribe in Steam, wait for the download to finish, then try again.")
		openURL("https://steamcommunity.com/sharedfiles/filedetails/?id=" + workshopExt)
		return
	}
	extJar := findExtensionsJar()
	if extJar == "" {
		appendLog("ERROR: Workshop " + workshopExt + " downloaded, but no ZombieBuddy.jar found under mods\\" + extModID + ".")
		return
	}
	dll := findWorkshopFile(workshopZB, "zbNative.dll")
	if dll == "" {
		appendLog("ERROR: zbNative.dll not found in original ZombieBuddy workshop folder (" + workshopZB + "). Run step 1 and subscribe first.")
		return
	}

	for _, name := range []string{"ZombieBuddy.jar", "zbNative.dll"} {
		if !backupGameFile(name, backupDir, stamp) {
			return
		}
	}
	for _, c := range []struct{ src, name string }{{extJar, "ZombieBuddy.jar"}, {dll, "zbNative.dll"}} {
		dst := filepath.Join(pzRoot, c.name)
		if err := copyFile(c.src, dst); err != nil {
			appendLog("ERROR writing " + c.name + ": " + err.Error())
			appendLog("Previous files backed up to " + backupDir + " (suffix ." + stamp + ".bak).")
			return
		}
		sum, _ := fileSHA256(dst)
		appendLog("Installed " + c.name + " from Extensions (SHA-256 " + sum + ")")
	}
	appendLog("Done. ZombieBuddy.jar is now the Extensions version. Keep original ZombieBuddy subscribed.")
	appendLog("Note: a newer official ZombieBuddy update may overwrite this JAR — that is expected.")
	showInfo("ZombieBuddy.jar swapped to the Extensions version.\n\nBackups saved to:\n" + backupDir)
}

// restoreJarToOfficial puts back the official ZombieBuddy.jar and zbNative.dll
// sourced directly from the ZombieBuddy Workshop folder (no backup required).
func restoreJarToOfficial() {
	backupDir, stamp, ok := jarSwapPrecheck("Restore JAR → Official ZombieBuddy")
	if !ok {
		return
	}

	officialJar := findWorkshopFile(workshopZB, "ZombieBuddy.jar")
	if officialJar == "" {
		appendLog("ERROR: ZombieBuddy.jar not found in original ZombieBuddy workshop folder (" + workshopZB + ").")
		appendLog("Make sure ZombieBuddy is subscribed in Steam and has finished downloading.")
		return
	}
	dll := findWorkshopFile(workshopZB, "zbNative.dll")
	if dll == "" {
		appendLog("ERROR: zbNative.dll not found in original ZombieBuddy workshop folder (" + workshopZB + ").")
		return
	}

	for _, name := range []string{"ZombieBuddy.jar", "zbNative.dll"} {
		if !backupGameFile(name, backupDir, stamp) {
			return
		}
	}
	for _, c := range []struct{ src, name string }{{officialJar, "ZombieBuddy.jar"}, {dll, "zbNative.dll"}} {
		dst := filepath.Join(pzRoot, c.name)
		if err := copyFile(c.src, dst); err != nil {
			appendLog("ERROR writing " + c.name + ": " + err.Error())
			appendLog("Previous files backed up to " + backupDir + " (suffix ." + stamp + ".bak).")
			return
		}
		sum, _ := fileSHA256(dst)
		appendLog("Restored " + c.name + " from official ZombieBuddy (SHA-256 " + sum + ")")
	}
	appendLog("Done. ZombieBuddy.jar is now the official version from Workshop " + workshopZB + ".")
	showInfo("ZombieBuddy.jar restored to the official version.\n\nBackups saved to:\n" + backupDir)
}

func verifyInstallation() {
	detectSteamAndPZ()
	appendLog("--- Verification ---")
	if pzRoot == "" {
		appendLog("FAIL: Project Zomboid not found.")
		return
	}
	appendLog("PASS: Project Zomboid: " + pzRoot)
	for _, f := range []string{"ZombieBuddy.jar", "zbNative.dll"} {
		p := filepath.Join(pzRoot, f)
		if _, err := os.Stat(p); err == nil {
			sum, _ := fileSHA256(p)
			appendLog("PASS: " + f + " present (" + sum + ")")
		} else {
			appendLog("WARN: " + f + " is missing")
		}
	}
	for _, id := range []string{workshopZB, workshopView, workshopModels} {
		if p := findWorkshopDir(id); p != "" {
			appendLog("PASS: Workshop " + id + " downloaded: " + p)
		} else {
			appendLog("WARN: Workshop " + id + " not found locally")
		}
	}
	appendLog("INFO: " + extensionsStatus())
	appendLog("Verification complete. Use FINAL CHECK for the complete readiness test.")
}

func finalCheck() {
	detectSteamAndPZ()
	appendLog("")
	appendLog("========== FINAL INSTALLATION CHECK ==========")
	if pzRoot == "" {
		appendLog("FAIL  Project Zomboid installation was not found.")
		showError("Final check failed: Project Zomboid was not found.")
		return
	}

	failures := 0
	warnings := 0
	pass := func(label string) { appendLog("PASS  " + label) }
	fail := func(label string) { failures++; appendLog("FAIL  " + label) }
	warn := func(label string) { warnings++; appendLog("WARN  " + label) }

	pass("Project Zomboid found: " + pzRoot)

	// Core ZombieBuddy files. Current official Windows instructions require both.
	for _, f := range []string{"ZombieBuddy.jar", "zbNative.dll"} {
		p := filepath.Join(pzRoot, f)
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			sum, err := fileSHA256(p)
			if err == nil {
				pass(f + " present, SHA-256 " + sum)
			} else {
				fail(f + " could not be hashed")
			}
		} else {
			fail(f + " missing from the Project Zomboid game directory")
		}
	}

	// Required Workshop downloads. We verify actual local content, not merely the Steam URL.
	workshopChecks := []struct{ id, name, marker string }{
		{workshopZB, "ZombieBuddy", "ZombieBuddy"},
		{workshopView, "Project Viewpoint", "Viewpoint"},
		{workshopModels, "6244 3D models for Viewpoint", ""},
	}
	for _, w := range workshopChecks {
		root := findWorkshopDir(w.id)
		if root == "" {
			fail(w.name + " Workshop content is not downloaded")
		} else if hasWorkshopContent(root, w.marker) {
			pass(w.name + " Workshop content found")
		} else {
			warn(w.name + " Workshop folder exists, but its expected mod files were not found")
		}
	}

	// ZombieBuddy Extensions is optional: report its state, never fail or warn on it.
	appendLog("INFO  " + extensionsStatus())

	// Check the normal launcher JSON for the current official Windows agent argument.
	jsonPath := filepath.Join(pzRoot, "ProjectZomboid64.json")
	if containsText(jsonPath, "-agentlib:zbNative") {
		pass("ProjectZomboid64.json contains the ZombieBuddy Windows agent launch option")
	} else {
		warn("ProjectZomboid64.json does not contain -agentlib:zbNative; run the official ZombieBuddy installer and choose Both/Normal Launch")
	}

	// Check the alternate launcher as well when present.
	batPath := filepath.Join(pzRoot, "ProjectZomboid64.bat")
	if _, err := os.Stat(batPath); err == nil {
		if containsText(batPath, "-agentlib:zbNative") {
			pass("ProjectZomboid64.bat contains the ZombieBuddy Windows agent option")
		} else {
			warn("ProjectZomboid64.bat does not contain -agentlib:zbNative (alternate launch may not be configured)")
		}
	}

	// Check the user's Zomboid mods directory for the Workshop mods when Steam exposes them there.
	modsRoot := filepath.Join(os.Getenv("USERPROFILE"), "Zomboid", "mods")
	if st, err := os.Stat(modsRoot); err == nil && st.IsDir() {
		if findModInfo(modsRoot, "Viewpoint") {
			pass("Viewpoint mod is visible in the user's Zomboid mods directory")
		}
	}

	appendLog("----------------------------------------------")
	if failures == 0 && warnings == 0 {
		appendLog("SUCCESS  Everything checked out. Project Viewpoint is READY TO LAUNCH.")
		appendLog("Launch Project Zomboid and look for the ZombieBuddy indicator, then press O to test Viewpoint.")
		showInfo("FINAL CHECK PASSED\n\nProject Viewpoint is ready to launch.")
	} else if failures == 0 {
		appendLog(fmt.Sprintf("READY WITH WARNINGS  %d warning(s). Review the lines above before launching.", warnings))
		showInfo(fmt.Sprintf("Final check completed with %d warning(s).\n\nReview the log before launching.", warnings))
	} else {
		appendLog(fmt.Sprintf("NOT READY  %d failure(s), %d warning(s). Fix the failed checks and run FINAL CHECK again.", failures, warnings))
		showError(fmt.Sprintf("Final check failed.\n\n%d failure(s) and %d warning(s).\n\nSee the log for details.", failures, warnings))
	}
}

func containsText(path, needle string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(b)), strings.ToLower(needle))
}

func hasWorkshopContent(root, marker string) bool {
	if marker == "" {
		entries, err := os.ReadDir(root)
		return err == nil && len(entries) > 0
	}
	found := false
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(marker)) || strings.EqualFold(info.Name(), "mod.info") {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func findModInfo(root, marker string) bool {
	found := false
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.EqualFold(info.Name(), "mod.info") {
			return nil
		}
		if containsText(path, marker) {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func detectSteamAndPZ() {
	// Common Steam locations first; then parse libraryfolders.vdf.
	candidates := []string{}
	if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
		candidates = append(candidates, filepath.Join(pf, "Steam"))
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		candidates = append(candidates, filepath.Join(pf, "Steam"))
	}
	if ld := os.Getenv("LocalAppData"); ld != "" {
		candidates = append(candidates, filepath.Join(ld, "Steam"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "steam.exe")); err == nil {
			steamRoot = c
			break
		}
	}
	if steamRoot == "" {
		return
	}
	libs := []string{steamRoot}
	vdf := filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf")
	if b, err := os.ReadFile(vdf); err == nil {
		re := regexp.MustCompile(`"path"\s+"([^"]+)"`)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if len(m) > 1 {
				libs = append(libs, strings.ReplaceAll(m[1], `\\`, `\`))
			}
		}
	}
	for _, lib := range libs {
		p := filepath.Join(lib, "steamapps", "common", "ProjectZomboid")
		if _, err := os.Stat(filepath.Join(p, "ProjectZomboid64.json")); err == nil {
			pzRoot = p
			return
		}
		if _, err := os.Stat(p); err == nil {
			if pzRoot == "" {
				pzRoot = p
			}
		}
	}
}

func findWorkshopDir(id string) string {
	if steamRoot == "" {
		detectSteamAndPZ()
	}
	roots := []string{steamRoot}
	// Also use the library containing PZ if it differs.
	if pzRoot != "" {
		parts := strings.Split(filepath.Clean(pzRoot), string(os.PathSeparator))
		for i := range parts {
			if strings.EqualFold(parts[i], "steamapps") && i > 0 {
				roots = append(roots, strings.Join(parts[:i], string(os.PathSeparator)))
			}
		}
	}
	seen := map[string]bool{}
	for _, r := range roots {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		p := filepath.Join(r, "steamapps", "workshop", "content", pzAppID, id)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return ""
}

func findWorkshopFile(id, name string) string {
	root := findWorkshopDir(id)
	if root == "" {
		return ""
	}
	var found string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && strings.EqualFold(info.Name(), name) {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// findExtensionsJar locates the Extensions replacement JAR. The author documents it at
// <workshop>\3807686870\mods\ZombieBuddy_Extensions\42.21\ZombieBuddy.jar; a JAR in a
// "42.21" folder is preferred, otherwise any ZombieBuddy.jar under the mod folder is used.
func findExtensionsJar() string {
	root := findWorkshopDir(workshopExt)
	if root == "" {
		return ""
	}
	var preferred, fallback string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.EqualFold(info.Name(), "ZombieBuddy.jar") {
			return nil
		}
		low := strings.ToLower(path)
		if !strings.Contains(low, strings.ToLower(extModID)) {
			return nil
		}
		if strings.Contains(low, "42.21") && preferred == "" {
			preferred = path
		} else if fallback == "" {
			fallback = path
		}
		return nil
	})
	if preferred != "" {
		return preferred
	}
	return fallback
}

// extensionsStatus describes the optional Extensions add-on state for the log.
func extensionsStatus() string {
	src := findExtensionsJar()
	if src == "" {
		return "ZombieBuddy Extensions (optional) not downloaded — not required."
	}
	srcHash, err1 := fileSHA256(src)
	dstHash, err2 := fileSHA256(filepath.Join(pzRoot, "ZombieBuddy.jar"))
	if err1 == nil && err2 == nil && strings.EqualFold(srcHash, dstHash) {
		return "ZombieBuddy Extensions (optional) is installed."
	}
	return "ZombieBuddy Extensions (optional) is downloaded but not installed — using official ZombieBuddy."
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Kept for future GitHub API based self-update support. The current build intentionally
// opens the official release page instead of silently executing a newly downloaded binary.
type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}
type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func fetchReleaseInfo() (*releaseInfo, error) {
	req, err := http.NewRequest("GET", githubReleaseAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Project-Viewpoint-Installer")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub HTTP %s", resp.Status)
	}
	var r releaseInfo
	err = json.NewDecoder(resp.Body).Decode(&r)
	return &r, err
}
