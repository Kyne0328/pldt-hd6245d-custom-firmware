package printing

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type output struct{ bytes.Buffer }

func (o *output) Close() error { return nil }
func TestPrinterValidation(t *testing.T) {
	valid := Options{Listen: "127.0.0.1:19100", AllowedCIDR: "127.0.0.0/8", MaxBytes: 1024, MaxJobs: 1, Duration: 2 * time.Second}
	if _, _, err := ValidateOptions(valid); err != nil {
		t.Fatal(err)
	}
	invalid := []Options{
		{Listen: "0.0.0.0:9100", AllowedCIDR: "0.0.0.0/0", MaxBytes: 1024, MaxJobs: 1, Duration: time.Second},
		{Listen: "192.0.2.1:9100", AllowedCIDR: "192.0.2.0/24", MaxBytes: 1024, MaxJobs: 1, Duration: time.Second},
		{Listen: "127.0.0.1:9100", AllowedCIDR: "192.168.1.0/24", MaxBytes: 1024, MaxJobs: 1, Duration: time.Second},
		{Listen: "127.0.0.1:9100", AllowedCIDR: "127.0.0.0/8", MaxBytes: 5 * 1024 * 1024, MaxJobs: 1, Duration: time.Second},
	}
	for _, v := range invalid {
		if _, _, err := ValidateOptions(v); err == nil {
			t.Fatalf("accepted unsafe printer bind: %+v", v)
		}
	}
}
func TestPrintJobIsBufferedAndBounded(t *testing.T) {
	var printed output
	op := func() (io.WriteCloser, error) { return &printed, nil }
	srv, cli := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- receive(srv, 256, op) }()
	_, _ = cli.Write([]byte("test-page"))
	_ = cli.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if printed.String() != "test-page" {
		t.Fatalf("incorrect raw job: %q", printed.String())
	}
	printed.Reset()
	srv, cli = net.Pipe()
	go func() { done <- receive(srv, 5, op) }()
	_, _ = cli.Write([]byte("longer-than-five"))
	_ = cli.Close()
	if err := <-done; err == nil {
		t.Fatal("oversized job printed")
	}
	if printed.Len() != 0 {
		t.Fatal("partial oversized job delivered")
	}
}
func TestPrinterDeviceFixture(t *testing.T) {
	base := t.TempDir()
	class := filepath.Join(base, "sys", "class", "usb")
	dev := filepath.Join(base, "dev")
	deviceDir := filepath.Join(base, "sys", "devices", "usb1", "usb", "lp0")
	if err := os.MkdirAll(deviceDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(class, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(deviceDir, filepath.Join(class, "lp0")); err != nil {
		t.Fatal(err)
	}
	if len(List(class, dev)) != 0 {
		t.Fatal("fabricated USB printer from sysfs alone without character device")
	}
}
func TestServerNoListenerOnInvalidOptions(t *testing.T) {
	o := Options{Listen: "0.0.0.0:9100", AllowedCIDR: "0.0.0.0/0", Duration: time.Second, MaxBytes: 100, MaxJobs: 1, OpenDevice: func() (io.WriteCloser, error) { return &output{}, nil }}
	if err := Serve(context.Background(), o); err == nil {
		t.Fatal("unsafe bind enabled")
	}
}
func TestLoopbackPrintServer(t *testing.T) {
	hold, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := hold.Addr().(*net.TCPAddr).Port
	_ = hold.Close()
	done := make(chan error, 1)
	out := &output{}
	opts := Options{Listen: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), AllowedCIDR: "127.0.0.0/8",
		MaxBytes: 4096, MaxJobs: 1, Duration: 4 * time.Second, OpenDevice: func() (io.WriteCloser, error) { return out, nil }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- Serve(ctx, opts) }()
	var conn net.Conn
	for i := 0; i < 50; i++ {
		conn, err = net.DialTimeout("tcp", opts.Listen, 150*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(conn, strings.NewReader("printer-test")); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if out.String() != "printer-test" {
		t.Fatalf("not transmitted: %s", out.String())
	}
}
