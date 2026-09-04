package desktopapps

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadLocalizedDesktopEntry(t *testing.T) {
	t.Setenv("LC_ALL", "es_CL.UTF-8")
	directory := t.TempDir()
	path := filepath.Join(directory, "example.desktop")
	data := "[Desktop Entry]\nType=Application\nName=Settings\nName[es]=Ajustes\nComment=Demo\nIcon=example\nExec=example --open\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	application, ok := Read(path, "example.desktop", directory)
	if !ok {
		t.Fatal("desktop entry was rejected")
	}
	if application.Name != "Ajustes" || application.Icon != "example" || application.Path != path {
		t.Fatalf("unexpected application: %#v", application)
	}
}

func TestReadRejectsHiddenAndUnavailableApplications(t *testing.T) {
	directory := t.TempDir()
	for name, extra := range map[string]string{
		"hidden.desktop":  "Hidden=true\n",
		"missing.desktop": "TryExec=definitely-not-a-real-panelpc-command\n",
	} {
		path := filepath.Join(directory, name)
		data := "[Desktop Entry]\nType=Application\nName=Example\nExec=example\n" + extra
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := Read(path, name, directory); ok {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestLaunchCommandUsesDesktopNativeLaunchers(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/apps/share")
	t.Setenv("XDG_DATA_DIRS", "/usr/share")
	path := "/apps/share/applications/com.example.App.desktop"
	found := func(name string) string {
		available := map[string]string{"kstart5": "/usr/bin/kstart5", "gtk-launch": "/usr/bin/gtk-launch", "gio": "/usr/bin/gio"}
		return available[name]
	}
	program, arguments, ok := launchCommand(path, "KDE", found)
	if !ok || program != "/usr/bin/kstart5" || !reflect.DeepEqual(arguments, []string{"--application", "com.example.App"}) {
		t.Fatalf("KDE command = %q %#v, %v", program, arguments, ok)
	}
	program, arguments, ok = launchCommand(path, "GNOME", found)
	if !ok || program != "/usr/bin/gtk-launch" || !reflect.DeepEqual(arguments, []string{"com.example.App"}) {
		t.Fatalf("GNOME command = %q %#v, %v", program, arguments, ok)
	}
	program, arguments, ok = launchCommand("/home/test/Desktop/example.desktop", "COSMIC", found)
	if !ok || program != "/usr/bin/gio" || !reflect.DeepEqual(arguments, []string{"launch", "/home/test/Desktop/example.desktop"}) {
		t.Fatalf("fallback command = %q %#v, %v", program, arguments, ok)
	}
}

func TestExternalEnvironmentRestoresHostLibrariesAndDropsActivationTokens(t *testing.T) {
	result := externalEnvironment([]string{
		"PATH=/usr/bin",
		"LD_LIBRARY_PATH=/tmp/appimage",
		"LD_LIBRARY_PATH_ORIG=/usr/local/lib",
		"XDG_ACTIVATION_TOKEN=used",
		"DESKTOP_STARTUP_ID=used",
	})
	want := []string{"LD_LIBRARY_PATH=/usr/local/lib", "PATH=/usr/bin"}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("environment = %#v, want %#v", result, want)
	}
}
