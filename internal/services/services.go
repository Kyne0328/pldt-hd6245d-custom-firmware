package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
)

type Definition struct {
	Name    string   `json:"name"`
	Exec    string   `json:"exec"`
	Args    []string `json:"args"`
	Enabled bool     `json:"enabled"`
	Restart bool     `json:"restart"`
}
type Config struct {
	Services []Definition `json:"services"`
}
type State struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	PID       int    `json:"pid"`
	Restarts  int    `json:"restarts"`
	LastError string `json:"lastError,omitempty"`
}
type Supervisor struct {
	mu     sync.RWMutex
	states map[string]State
}

func New() *Supervisor { return &Supervisor{states: map[string]State{}} }
func ReadConfig(path string) (Config, error) {
	f, e := os.Open(path)
	if e != nil {
		return Config{}, e
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	var c Config
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	seen := map[string]bool{}
	for _, s := range c.Services {
		if s.Name == "" || len(s.Name) > 64 || seen[s.Name] {
			return c, errors.New("duplicate or invalid service name")
		}
		if !filepath.IsAbs(s.Exec) || filepath.Clean(s.Exec) != s.Exec {
			return c, fmt.Errorf("%s: executable must be absolute and clean", s.Name)
		}
		if len(s.Args) > 24 {
			return c, errors.New("too many arguments")
		}
		seen[s.Name] = true
	}
	return c, nil
}
func (s *Supervisor) set(st State) { s.mu.Lock(); s.states[st.Name] = st; s.mu.Unlock() }
func (s *Supervisor) Snapshot() []State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []State{}
	for _, v := range s.states {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (s *Supervisor) Run(ctx context.Context, c Config) {
	var wg sync.WaitGroup
	for _, d := range c.Services {
		s.set(State{Name: d.Name, State: "disabled"})
		if !d.Enabled {
			continue
		}
		wg.Add(1)
		go func(def Definition) { defer wg.Done(); s.run(ctx, def) }(d)
	}
	<-ctx.Done()
	wg.Wait()
}
func (s *Supervisor) run(ctx context.Context, d Definition) {
	st := State{Name: d.Name, State: "starting"}
	s.set(st)
	for {
		if ctx.Err() != nil {
			st.State = "stopped"
			st.PID = 0
			s.set(st)
			return
		}
		cmd := exec.Command(d.Exec, d.Args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if e := cmd.Start(); e != nil {
			st.State = "failed"
			st.LastError = e.Error()
			st.PID = 0
			s.set(st)
		} else {
			st.PID = cmd.Process.Pid
			st.State = "running"
			st.LastError = ""
			s.set(st)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case e := <-done:
				st.PID = 0
				st.State = "exited"
				if e != nil {
					st.LastError = e.Error()
				}
				s.set(st)
			case <-ctx.Done():
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					<-done
				}
				st.State = "stopped"
				st.PID = 0
				s.set(st)
				return
			}
		}
		if !d.Restart || st.Restarts >= 5 {
			return
		}
		st.Restarts++
		st.State = "restarting"
		s.set(st)
		select {
		case <-ctx.Done():
			st.State = "stopped"
			s.set(st)
			return
		case <-time.After(time.Duration(st.Restarts) * time.Second):
		}
	}
}
