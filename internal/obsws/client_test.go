package obsws

import (
	"bufio"
	"bytes"
	"testing"
)

func TestAuthentication(t *testing.T) {
	got := authentication("password", "salt", "challenge")
	const want = "zTM5ki6L2vVvBQiTG9ckH1Lh64AbnCf6XZ226UmnkIA="
	if got != want {
		t.Fatalf("authentication = %q, want %q", got, want)
	}
}

func TestFrameRoundTrip(t *testing.T) {
	var wire bytes.Buffer
	payload := []byte(`{"op":6,"d":{"requestType":"GetVersion"}}`)
	if err := writeFrame(&wire, 1, payload); err != nil {
		t.Fatal(err)
	}
	opcode, got, err := readFrame(bufio.NewReader(bytes.NewReader(wire.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	if opcode != 1 || string(got) != string(payload) {
		t.Fatalf("frame = opcode %d %q", opcode, got)
	}
}
