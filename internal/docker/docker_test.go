package docker

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func frame(stream byte, s string) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(s)))
	return append(h, s...)
}

func TestScanDockerStreamMultiplexed(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(frame(1, "line one\nline "))
	buf.Write(frame(2, "stderr ignored\n"))
	buf.Write(frame(1, "two\n"))
	var got []string
	_ = ScanStream(&buf, func(l string) { got = append(got, l) })
	if len(got) != 2 || got[0] != "line one" || got[1] != "line two" {
		t.Fatalf("got %q", got)
	}
}

func TestScanDockerStreamRawTTY(t *testing.T) {
	var got []string
	_ = ScanStream(bytes.NewBufferString("abcdefgh\nsecond\n"), func(l string) { got = append(got, l) })
	if len(got) != 2 {
		t.Fatalf("got %q", got)
	}
}
