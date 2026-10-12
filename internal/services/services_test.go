package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "services.json")
	os.WriteFile(p, []byte(`{"services":[{"name":"a","exec":"/bin/sleep","args":["30"],"enabled":false}]}`), 0600)
	cfg, e := ReadConfig(p)
	if e != nil || len(cfg.Services) != 1 {
		t.Fatal(e)
	}
	os.WriteFile(p, []byte(`{"services":[{"name":"a","exec":"sh"},{"name":"a","exec":"/bin/echo"}]}`), 0600)
	if _, e := ReadConfig(p); e == nil {
		t.Fatal("unsafe config accepted")
	}
}
func TestSupervisorLifecycle(t *testing.T) {
	if _, e := os.Stat("/bin/sleep"); e != nil {
		t.Skip("/bin/sleep missing")
	}
	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool)
	go func() {
		s.Run(ctx, Config{Services: []Definition{{Name: "clock", Exec: "/bin/sleep", Args: []string{"60"}, Enabled: true}}})
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	st := s.Snapshot()
	if len(st) != 1 || st[0].State != "running" {
		t.Fatal("service failed to start", st)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("supervisor failed to stop")
	}
	st = s.Snapshot()
	if st[0].State != "stopped" {
		t.Fatal("service leaked", st)
	}
}
