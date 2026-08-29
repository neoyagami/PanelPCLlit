package audio

import "testing"

func TestMatchAppsCaseInsensitive(t *testing.T) {
	apps := []App{{ID: 1, Name: "Spotify"}, {ID: 2, Name: "Firefox Web Content"}, {ID: 3, Name: "firefox"}}
	got := matchApps(apps, "FIREfox")
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("unexpected matches: %#v", got)
	}
	if got := matchApps(apps, ""); got != nil {
		t.Fatalf("an empty target must return nil: %#v", got)
	}
}

func TestParseDevicesExcludesMonitorSources(t *testing.T) {
	data := []byte(`[
		{"name":"alsa_input.usb-mic","description":"USB microphone","state":"RUNNING","monitor_source":""},
		{"name":"alsa_output.hdmi.monitor","description":"Monitor of HDMI","state":"IDLE","monitor_source":"alsa_output.hdmi"}
	]`)
	devices, err := parseDevices(data, "input")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != "alsa_input.usb-mic" || devices[0].Name != "USB microphone" {
		t.Fatalf("unexpected devices: %#v", devices)
	}
}

func TestParseOutputKeepsMonitorForVU(t *testing.T) {
	data := []byte(`[{"name":"alsa_output.hdmi","description":"HDMI","state":"IDLE","monitor_source":"alsa_output.hdmi.monitor"}]`)
	devices, err := parseDevices(data, "output")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Monitor != "alsa_output.hdmi.monitor" {
		t.Fatalf("unexpected monitor: %#v", devices)
	}
}
