package device

import "testing"

func TestParseReport(t *testing.T) {
	tests := []struct {
		data []byte
		want Event
		ok   bool
	}{
		{[]byte{1, 2, 255}, Event{Kind: "turn", Knob: 2, Value: 255}, true},
		{[]byte{2, 0, 1}, Event{Kind: "press", Knob: 0, Value: 1}, true},
		{[]byte{1, 4, 10}, Event{}, false},
		{[]byte{9, 0, 0}, Event{}, false},
		{[]byte{1, 0}, Event{}, false},
	}
	for _, test := range tests {
		got, ok := ParseReport(test.data)
		if ok != test.ok || got != test.want {
			t.Errorf("ParseReport(%v) = %#v, %v; want %#v, %v", test.data, got, ok, test.want, test.ok)
		}
	}
}

func TestInjectIsBounded(t *testing.T) {
	m := NewManager()
	for i := 0; i < 1000; i++ {
		m.Inject(Event{Kind: "turn", Knob: 0, Value: i % 256})
	}
	if got := len(m.events); got != cap(m.events) {
		t.Fatalf("queue = %d, capacity = %d", got, cap(m.events))
	}
}

func TestMiniLightingReport(t *testing.T) {
	lighting := Lighting{GlobalBrightness: 50}
	lighting.Knobs[0] = KnobLight{Color: "#ff8040"}
	lighting.Knobs[1] = KnobLight{Color: "#00ff00", TrackValue: true}
	report := BuildMiniLightingReport(lighting)
	if len(report) != 64 {
		t.Fatalf("length = %d", len(report))
	}
	wantStatic := []byte{6, 2, 1, 127, 64, 32, 0, 0, 0}
	for i, want := range wantStatic {
		if report[i] != want {
			t.Fatalf("report[%d] = %d, want %d; report=%v", i, report[i], want, report[:16])
		}
	}
	offset := 2 + 7
	wantTrack := []byte{2, 0, 0, 0, 0, 127, 0}
	for i, want := range wantTrack {
		if report[offset+i] != want {
			t.Fatalf("track[%d] = %d, want %d", i, report[offset+i], want)
		}
	}
}

func TestLightingQueueKeepsLatest(t *testing.T) {
	m := NewManager()
	for i := 0; i < 100; i++ {
		m.SetLighting(Lighting{GlobalBrightness: i})
	}
	if got := len(m.lighting); got != 1 {
		t.Fatalf("RGB queue = %d, want 1", got)
	}
	if got := (<-m.lighting).GlobalBrightness; got != 99 {
		t.Fatalf("brightness = %d, want 99", got)
	}
}
