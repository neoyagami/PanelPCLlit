package audio

import "testing"

func TestMatchAppsCaseInsensitive(t *testing.T) {
	apps := []App{{ID: 1, Name: "Spotify"}, {ID: 2, Name: "Firefox Web Content"}, {ID: 3, Name: "firefox"}}
	got := matchApps(apps, "FIREfox")
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("matches inesperados: %#v", got)
	}
	if got := matchApps(apps, ""); got != nil {
		t.Fatalf("target vacío debe devolver nil: %#v", got)
	}
}

func TestParseDevicesExcludesMonitorSources(t *testing.T) {
	data := []byte(`[
		{"name":"alsa_input.usb-mic","description":"Micrófono USB","state":"RUNNING","monitor_source":""},
		{"name":"alsa_output.hdmi.monitor","description":"Monitor of HDMI","state":"IDLE","monitor_source":"alsa_output.hdmi"}
	]`)
	devices, err := parseDevices(data, "input")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != "alsa_input.usb-mic" || devices[0].Name != "Micrófono USB" {
		t.Fatalf("dispositivos inesperados: %#v", devices)
	}
}

func TestParseOutputKeepsMonitorForVU(t *testing.T) {
	data := []byte(`[{"name":"alsa_output.hdmi","description":"HDMI","state":"IDLE","monitor_source":"alsa_output.hdmi.monitor"}]`)
	devices, err := parseDevices(data, "output")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Monitor != "alsa_output.hdmi.monitor" {
		t.Fatalf("monitor inesperado: %#v", devices)
	}
}
