package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/admin"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/services"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/storage"
)

const defaultServices = "config/services.json"

func usage() {
	fmt.Fprintln(os.Stderr, "HGFW — independent router userspace (engineering build)")
	fmt.Fprintln(os.Stderr, "Commands: init, serve, status, network-plan, usb, usb-mount, usb-unmount, version")
	fmt.Fprintln(os.Stderr, "No command changes NAND, NVRAM, bridge, Wi-Fi, or firewall.")
}
func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("command required")
	}
	switch args[0] {
	case "version":
		fmt.Println("HGFW stage2 armv7 development — no flash support")
		return nil
	case "status":
		return json.NewEncoder(os.Stdout).Encode(device.Snapshot())
	case "usb":
		return json.NewEncoder(os.Stdout).Encode(device.USBDisks("/sys/class/block", "/proc/mounts"))
	case "network-plan":
		f := flag.NewFlagSet("network-plan", flag.ContinueOnError)
		p := f.String("config", "config/network.example.json", "desired topology")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		plan, e := device.ReadNetworkPlan(*p)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(plan)
	case "init":
		f := flag.NewFlagSet("init", flag.ContinueOnError)
		passwd := f.String("password-file", "", "file containing a chosen admin password")
		out := f.String("auth-file", "private/admin.json", "destination credential file (must not already exist)")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if *passwd == "" {
			return errors.New("--password-file required; passwords must never appear on command line")
		}
		in, e := os.ReadFile(*passwd)
		if e != nil {
			return e
		}
		cred, e := admin.NewCredential(strings.TrimRight(string(in), "\r\n"))
		if e != nil {
			return e
		}
		if e = admin.SaveCredential(*out, cred); e != nil {
			return e
		}
		fmt.Println("Created protected admin credential file:", *out)
		return nil
	case "serve":
		f := flag.NewFlagSet("serve", flag.ContinueOnError)
		addr := f.String("listen", "127.0.0.1:9080", "local admin address")
		authPath := f.String("auth-file", "private/admin.json", "protected credential")
		svcPath := f.String("services", defaultServices, "explicit supervised-service list")
		cert := f.String("tls-cert", "", "HTTPS server certificate path")
		key := f.String("tls-key", "", "HTTPS private key path")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if e := admin.SecureListen(*addr, *cert, *key); e != nil {
			return e
		}
		cred, e := admin.LoadCredential(*authPath)
		if e != nil {
			return fmt.Errorf("admin not initialized; run hgfw init: %w", e)
		}
		defs, e := services.ReadConfig(*svcPath)
		if e != nil {
			return e
		}
		sup := services.New()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		go sup.Run(ctx, defs)
		srv := &http.Server{Addr: *addr, Handler: admin.New(cred, sup).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
		result := make(chan error, 1)
		go func() {
			if *cert != "" {
				result <- srv.ListenAndServeTLS(*cert, *key)
			} else {
				result <- srv.ListenAndServe()
			}
		}()
		fmt.Println("HGFW admin started on", *addr, "(authenticated, no network configuration writes)")
		select {
		case err := <-result:
			stop()
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutdown)
		}
	case "usb-mount":
		f := flag.NewFlagSet("usb-mount", flag.ContinueOnError)
		dev := f.String("partition", "", "USB block partition e.g. /dev/sda1")
		root := f.String("mount-root", "/mnt/hgfw", "pre-created dedicated mount root")
		confirm := f.Bool("confirm", false, "explicit local permission to mount read-only")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if !*confirm {
			return errors.New("no changes performed; --confirm needed for read-only mount")
		}
		target, e := storage.MountReadOnly(*dev, *root)
		if e == nil {
			fmt.Println("Mounted read-only:", target)
		}
		return e
	case "usb-unmount":
		f := flag.NewFlagSet("usb-unmount", flag.ContinueOnError)
		path := f.String("path", "", "HGFW mount directory")
		root := f.String("mount-root", "/mnt/hgfw", "dedicated mount root")
		confirm := f.Bool("confirm", false, "explicit local permission to unmount")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if !*confirm {
			return errors.New("--confirm required")
		}
		return storage.Unmount(*path, *root)
	default:
		usage()
		return errors.New("unknown command " + args[0])
	}
}
func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "hgfw:", e)
		os.Exit(1)
	}
}
