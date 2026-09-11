package procnet

import "testing"

func TestDecodeHexAddr(t *testing.T) {
	if got := DecodeHexAddr("0100007F"); got == nil || got.String() != "127.0.0.1" {
		t.Fatalf("DecodeHexAddr(IPv4 loopback) = %v", got)
	}
	if got := DecodeHexAddr("bogus"); got != nil {
		t.Fatalf("DecodeHexAddr(invalid) = %v, want nil", got)
	}
}

func TestDecodeHexPort(t *testing.T) {
	if got := DecodeHexPort("0050"); got != 80 {
		t.Fatalf("DecodeHexPort(0050) = %d, want 80", got)
	}
}
