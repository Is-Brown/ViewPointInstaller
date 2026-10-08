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
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const (
	appName          = "Project Viewpoint Setup Assistant"
	pzAppID          = "108600"
	workshopView     = "3809306528"
	workshopZB       = "3619862853"
	workshopFix      = "3807686870"
	workshopModels   = "3810302175"
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

func openURL(url string) {
	if err := exec.Command("cmd", "/c", "start", "", url).Start(); err != nil {
		appendLog("ERROR opening URL: " + err.Error())
	}
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
						Text:       "Opens the required Workshop items, installs official ZombieBuddy, applies the B42.21 fix when needed, and verifies the result.",
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
							PushButton{Text: "3 · Apply B42.21 fix / extension", Font: btnFont, MinSize: btnMin,
								OnClicked: func() { go applyFix() }},
							PushButton{Text: "4 · Verify installation", Font: btnFont, MinSize: btnMin,
								OnClicked: func() { go verifyInstallation() }},
							PushButton{Text: "Open Project Zomboid folder", Font: btnFont, MinSize: btnMin,
								OnClicked: openPZFolder},
							PushButton{Text: "5 · FINAL CHECK — Ready to play", Font: btnFontBold, MinSize: btnMin,
								OnClicked: func() { go finalCheck() }},
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
		{workshopFix, "ZombieBuddy B42.21 fix / Extensions"},
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

func applyFix() {
	if pzRoot == "" {
		appendLog("ERROR: Project Zomboid installation was not found.")
		return
	}
	src := findFixJar()
	if src == "" {
		appendLog("ERROR: No B42.21 replacement ZombieBuddy.jar found in Workshop ID " + workshopFix + ".")
		return
	}
	dst := filepath.Join(pzRoot, "ZombieBuddy.jar")
	backupDir := filepath.Join(pzRoot, "ViewpointInstallerBackups")
	os.MkdirAll(backupDir, 0755)
	if _, err := os.Stat(dst); err == nil {
		backup := filepath.Join(backupDir, "ZombieBuddy.jar."+time.Now().Format("20060102-150405")+".bak")
		if err := copyFile(dst, backup); err != nil {
			appendLog("ERROR backing up ZombieBuddy.jar: " + err.Error())
			return
		}
		appendLog("Backed up existing ZombieBuddy.jar to " + backup)
	}
	if err := copyFile(src, dst); err != nil {
		appendLog("ERROR copying replacement JAR: " + err.Error())
		return
	}
	sum, _ := fileSHA256(dst)
	appendLog("Installed replacement ZombieBuddy.jar. SHA-256: " + sum)
	appendLog("Restart Project Zomboid and approve the Viewpoint Java mod when ZombieBuddy asks.")
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
	for _, id := range []string{workshopZB, workshopFix, workshopView, workshopModels} {
		if p := findWorkshopDir(id); p != "" {
			appendLog("PASS: Workshop " + id + " downloaded: " + p)
		} else {
			appendLog("WARN: Workshop " + id + " not found locally")
		}
	}
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
		{workshopFix, "ZombieBuddy B42.21 fix / Extensions", "ZombieBuddy_B42.21"},
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

	// Verify the B42.21 replacement actually matches the downloaded fix when possible.
	if src := findFixJar(); src != "" {
		dst := filepath.Join(pzRoot, "ZombieBuddy.jar")
		srcHash, srcErr := fileSHA256(src)
		dstHash, dstErr := fileSHA256(dst)
		if srcErr == nil && dstErr == nil && strings.EqualFold(srcHash, dstHash) {
			pass("Installed ZombieBuddy.jar matches the downloaded B42.21 replacement")
		} else if srcErr == nil && dstErr == nil {
			warn("ZombieBuddy.jar does not match the currently downloaded B42.21 replacement; this may be intentional if a newer official ZombieBuddy release is installed")
		}
	} else {
		warn("Could not locate a B42.21 replacement JAR to compare against")
	}

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

func findFixJar() string {
	root := findWorkshopDir(workshopFix)
	if root == "" {
		return ""
	}
	preferred := []string{"ZombieBuddy.jar"}
	for _, name := range preferred {
		var found string
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && info != nil && !info.IsDir() && strings.EqualFold(info.Name(), name) {
				low := strings.ToLower(path)
				if strings.Contains(low, "42.21") {
					found = path
					return filepath.SkipDir
				}
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	return ""
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
