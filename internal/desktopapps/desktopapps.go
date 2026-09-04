package desktopapps

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Application is a launchable freedesktop Desktop Entry. Path is persisted in
// profiles because it also supports shortcuts placed directly on the desktop.
type Application struct {
	DesktopID string `json:"desktopId"`
	Name      string `json:"name"`
	Icon      string `json:"icon,omitempty"`
	Path      string `json:"path"`
	Comment   string `json:"comment,omitempty"`
}

// Discover returns visible application entries using freedesktop precedence:
// the user's desktop and data directory before system data directories.
func Discover() ([]Application, error) {
	desktop := desktopDirectory()
	directories := append([]string{desktop}, applicationDirectories()...)
	seen := make(map[string]bool)
	applications := make([]Application, 0, 128)
	var firstErr error
	for _, directory := range uniquePaths(directories) {
		if directory == "" {
			continue
		}
		info, err := os.Stat(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() {
			if err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".desktop") {
				return nil
			}
			relative, err := filepath.Rel(directory, path)
			if err != nil {
				return nil
			}
			desktopID := strings.ReplaceAll(relative, string(filepath.Separator), "-")
			if seen[desktopID] {
				return nil
			}
			application, ok := Read(path, desktopID, desktop)
			if !ok {
				return nil
			}
			seen[desktopID] = true
			applications = append(applications, application)
			return nil
		})
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	sort.Slice(applications, func(i, j int) bool {
		left, right := strings.ToLower(applications[i].Name), strings.ToLower(applications[j].Name)
		if left == right {
			return applications[i].DesktopID < applications[j].DesktopID
		}
		return left < right
	})
	if len(applications) != 0 {
		return applications, nil
	}
	return applications, firstErr
}

// Read parses the subset of a Desktop Entry needed by the application picker.
func Read(path, desktopID, desktopDirectory string) (Application, bool) {
	entry, err := readDesktopEntry(path)
	if err != nil || entry["Type"] != "Application" && entry["Type"] != "" || truthy(entry["Hidden"]) {
		return Application{}, false
	}
	if truthy(entry["NoDisplay"]) && filepath.Clean(filepath.Dir(path)) != filepath.Clean(desktopDirectory) {
		return Application{}, false
	}
	if tryExec := strings.TrimSpace(entry["TryExec"]); tryExec != "" && findExecutable(tryExec) == "" {
		return Application{}, false
	}
	name := localized(entry, "Name")
	if name == "" || strings.TrimSpace(entry["Exec"]) == "" {
		return Application{}, false
	}
	if desktopID == "" {
		desktopID = filepath.Base(path)
	}
	return Application{
		DesktopID: desktopID,
		Name:      name,
		Icon:      strings.TrimSpace(entry["Icon"]),
		Path:      path,
		Comment:   localized(entry, "Comment"),
	}, true
}

// Launch uses the native desktop launcher selected by XDG_CURRENT_DESKTOP.
func Launch(path string) error {
	application, ok := Read(path, "", desktopDirectory())
	if !ok {
		return fmt.Errorf("the application shortcut is missing or invalid: %s", path)
	}
	program, arguments, ok := launchCommand(application.Path, os.Getenv("XDG_CURRENT_DESKTOP"), findExecutable)
	if !ok {
		return errors.New("no compatible desktop application launcher was found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, program, arguments...)
	command.Env = externalEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("application launcher timed out: %w", ctx.Err())
	}
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return fmt.Errorf("application launcher: %w: %s", err, detail)
		}
		return fmt.Errorf("application launcher: %w", err)
	}
	return nil
}

func launchCommand(path, currentDesktop string, lookPath func(string) string) (string, []string, bool) {
	desktopID := registeredDesktopID(path)
	desktops := make(map[string]bool)
	for _, name := range strings.FieldsFunc(currentDesktop, func(r rune) bool { return r == ':' || r == ';' }) {
		desktops[strings.ToLower(name)] = true
	}
	withoutSuffix := strings.TrimSuffix(desktopID, ".desktop")
	if desktopID != "" && desktops["kde"] {
		for _, candidate := range []string{"kstart6", "kstart", "kstart5"} {
			if program := lookPath(candidate); program != "" {
				return program, []string{"--application", withoutSuffix}, true
			}
		}
	}
	if desktopID != "" && desktops["gnome"] {
		if program := lookPath("gtk-launch"); program != "" {
			return program, []string{withoutSuffix}, true
		}
	}
	if program := lookPath("gio"); program != "" {
		return program, []string{"launch", path}, true
	}
	return "", nil, false
}

func registeredDesktopID(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	resolved, _ = filepath.Abs(resolved)
	for _, directory := range applicationDirectories() {
		base, err := filepath.Abs(directory)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(base, resolved)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return strings.ReplaceAll(relative, string(filepath.Separator), "-")
		}
	}
	parts := strings.Split(filepath.ToSlash(resolved), "/")
	for index := len(parts) - 2; index > 0; index-- {
		if parts[index-1] == "share" && parts[index] == "applications" {
			return strings.Join(parts[index+1:], "-")
		}
	}
	return ""
}

func applicationDirectories() []string {
	home, _ := os.UserHomeDir()
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" && home != "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	directories := []string{filepath.Join(dataHome, "applications")}
	for _, directory := range filepath.SplitList(dataDirs) {
		directories = append(directories, filepath.Join(directory, "applications"))
	}
	// Some autostart environments omit Flatpak exports from XDG_DATA_DIRS.
	if home != "" {
		directories = append(directories, filepath.Join(home, ".local", "share", "flatpak", "exports", "share", "applications"))
	}
	directories = append(directories, "/var/lib/flatpak/exports/share/applications")
	return uniquePaths(directories)
}

func desktopDirectory() string {
	home, _ := os.UserHomeDir()
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" && home != "" {
		configHome = filepath.Join(home, ".config")
	}
	file, err := os.Open(filepath.Join(configHome, "user-dirs.dirs"))
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			key, value, found := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
			if !found || key != "XDG_DESKTOP_DIR" {
				continue
			}
			value = strings.Trim(strings.TrimSpace(value), `"`)
			value = strings.ReplaceAll(value, "$HOME", home)
			if filepath.IsAbs(value) {
				return value
			}
		}
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Desktop")
}

func readDesktopEntry(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entry := make(map[string]string)
	inDesktopEntry := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inDesktopEntry = line == "[Desktop Entry]"
			continue
		}
		if !inDesktopEntry || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found {
			entry[strings.TrimSpace(key)] = unescape(strings.TrimSpace(value))
		}
	}
	return entry, scanner.Err()
}

func localized(entry map[string]string, key string) string {
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LC_MESSAGES")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	locale = strings.Split(strings.Split(locale, ".")[0], "@")[0]
	candidates := []string{}
	if locale != "" && locale != "C" && locale != "POSIX" {
		candidates = append(candidates, key+"["+locale+"]")
		if base := strings.Split(locale, "_")[0]; base != locale {
			candidates = append(candidates, key+"["+base+"]")
		}
	}
	candidates = append(candidates, key)
	for _, candidate := range candidates {
		if value := strings.TrimSpace(entry[candidate]); value != "" {
			return value
		}
	}
	return ""
}

func findExecutable(value string) string {
	if filepath.IsAbs(value) {
		if info, err := os.Stat(value); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return value
		}
		return ""
	}
	path, _ := exec.LookPath(value)
	return path
}

func externalEnvironment(source []string) []string {
	values := make(map[string]string, len(source))
	for _, item := range source {
		if key, value, found := strings.Cut(item, "="); found {
			values[key] = value
		}
	}
	delete(values, "XDG_ACTIVATION_TOKEN")
	delete(values, "DESKTOP_STARTUP_ID")
	if original, exists := values["LD_LIBRARY_PATH_ORIG"]; exists {
		values["LD_LIBRARY_PATH"] = original
	} else {
		delete(values, "LD_LIBRARY_PATH")
	}
	delete(values, "LD_LIBRARY_PATH_ORIG")
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

func truthy(value string) bool { return strings.EqualFold(strings.TrimSpace(value), "true") }

func unescape(value string) string {
	return strings.NewReplacer(`\s`, " ", `\n`, "\n", `\t`, "\t", `\r`, "\r", `\\`, `\`).Replace(value)
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, path)
	}
	return result
}
