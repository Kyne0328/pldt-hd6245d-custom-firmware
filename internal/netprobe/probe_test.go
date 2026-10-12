package netprobe

import "testing"

func TestLoopbackOnlyChild(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:9999", "127.0.0.2:9999", "192.168.1.2:9999", "[::1]:9999", "127.0.0.1:0", "127.0.0.1:70000"} {
		if validLoopback(addr) {
			t.Fatalf("unsafe address allowed: %s", addr)
		}
	}
	if !validLoopback("127.0.0.1:49152") {
		t.Fatal("safe IPv4 loopback rejected")
	}
}
