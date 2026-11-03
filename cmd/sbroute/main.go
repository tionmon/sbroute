package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tionmon/sbroute/internal/app"
	"github.com/tionmon/sbroute/internal/core"
	"github.com/tionmon/sbroute/internal/httpapi"
	"github.com/tionmon/sbroute/internal/model"
	"github.com/tionmon/sbroute/internal/store"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "SBRoute:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	command := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}
	if command == "version" {
		fmt.Printf("SBRoute %s (%s) · sing-box %s\n", model.Version, model.Commit, model.CoreVersion)
		return nil
	}
	if command == "help" {
		fmt.Println("sbroute <serve|check|import|export|admin|backup|restore|version> [flags]\nUse '<command> -h' for flags. Offline commands require the service to be stopped.")
		return nil
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	dir := f.String("data-dir", "./data", "private state directory")
	listen := f.String("listen", "127.0.0.1:8080", "management HTTP listen address")
	file := f.String("config", "", "YAML configuration path (check/import)")
	output := f.String("output", "", "output path (export/backup)")
	input := f.String("input", "", "backup database path (restore)")
	user := f.String("username", "admin", "administrator username")
	stdin := f.Bool("password-stdin", false, "read administrator password from stdin")
	secrets := f.Bool("secrets", false, "include credentials in exported configuration")
	trusted := f.String("trusted-proxy", "", "comma-separated reverse proxy IPs/CIDRs whose X-Forwarded-For/Proto are trusted (serve)")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	switch command {
	case "serve":
		proxies, err := parsePrefixes(*trusted)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		s, err := app.New(ctx, *dir)
		if err != nil {
			return err
		}
		defer s.Close()
		ln, err := net.Listen("tcp", *listen)
		if err != nil {
			return err
		}
		server := &http.Server{Handler: httpapi.NewWithOptions(s, httpapi.Options{TrustedProxies: proxies}), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
		errs := make(chan error, 1)
		go func() { errs <- server.Serve(ln) }()
		fmt.Printf("SBRoute %s · sing-box %s\nManagement: http://%s\nState: %s\n", model.Version, model.CoreVersion, ln.Addr(), *dir)
		select {
		case err := <-errs:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
			_ = server.Close()
			return nil
		}
	case "check":
		c, err := readConfig(*file)
		if err != nil {
			return err
		}
		e, err := core.New(context.Background(), nil)
		if err != nil {
			return err
		}
		defer e.Close()
		if err = e.Validate(c); err != nil {
			return err
		}
		fmt.Printf("Valid: %d nodes, %d groups, %d entrances\n", len(c.Nodes), len(c.Groups), len(c.Entrances))
		return nil
	case "admin":
		s, err := store.Open(*dir)
		if err != nil {
			return err
		}
		defer s.Close()
		var b []byte
		if *stdin {
			b, err = io.ReadAll(io.LimitReader(os.Stdin, 1024))
		} else {
			fmt.Fprint(os.Stderr, "Administrator password (12–72 bytes): ")
			b, err = term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
		}
		if err != nil {
			return err
		}
		password := strings.TrimRight(string(b), "\r\n")
		if err = httpapi.SetAdmin(s, *user, password); err != nil {
			return err
		}
		fmt.Println("Administrator saved; sessions are invalidated on service restart.")
		return nil
	case "import":
		c, err := readConfig(*file)
		if err != nil {
			return err
		}
		s, err := app.New(context.Background(), *dir)
		if err != nil {
			return err
		}
		defer s.Close()
		c.Version = s.Engine.Config().Version
		_, err = s.Apply(c)
		if err == nil {
			fmt.Println("Configuration imported")
		}
		return err
	case "export":
		s, err := store.Open(*dir)
		if err != nil {
			return err
		}
		defer s.Close()
		c, err := s.Load()
		if err != nil {
			return err
		}
		if !*secrets {
			c = c.Redacted()
		}
		b, err := yaml.Marshal(c)
		if err != nil {
			return err
		}
		if *output == "" {
			_, err = os.Stdout.Write(b)
			return err
		}
		return os.WriteFile(*output, b, 0600)
	case "backup":
		if *output == "" {
			return errors.New("--output required")
		}
		s, err := store.Open(*dir)
		if err != nil {
			return err
		}
		defer s.Close()
		absolute, err := filepath.Abs(*output)
		if err != nil {
			return err
		}
		return s.Backup(absolute)
	case "restore":
		return restore(*dir, *input)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}
func readConfig(path string) (model.Config, error) {
	c := model.Default()
	if path == "" {
		return c, errors.New("--config required")
	}
	f, e := os.Open(path)
	if e != nil {
		return c, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, model.MaxImportBytes+1))
	if e != nil || len(b) > model.MaxImportBytes {
		return c, errors.New("config unreadable or larger than 16MiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return c, errors.New("exactly one YAML document required")
	}
	return c, c.Validate()
}
func restore(dir, input string) error {
	if input == "" {
		return errors.New("--input required")
	}
	source, err := os.Open(input)
	if err != nil {
		return err
	}
	data := make([]byte, 16)
	_, err = io.ReadFull(source, data)
	source.Close()
	if err != nil || string(data) != "SQLite format 3\x00" {
		return errors.New("not a SQLite backup")
	}
	s, err := store.Open(dir)
	if err != nil {
		return err
	}
	defer s.Close()
	srcInfo, srcErr := os.Stat(input)
	dstInfo, dstErr := os.Stat(s.Path)
	if srcErr != nil || dstErr != nil {
		return errors.New("cannot stat source or destination")
	}
	if os.SameFile(srcInfo, dstInfo) {
		return errors.New("backup source must differ from the live database")
	}
	backup := filepath.Join(dir, "before-restore-"+time.Now().Format("20060102-150405")+".db")
	if err = s.Backup(backup); err != nil {
		return err
	}
	// Use SQLite's online backup via ATTACH + transaction, preserving the held lock.
	if _, err = s.DB.Exec(`ATTACH DATABASE ? AS restored`, input); err != nil {
		return err
	}
	defer func() { _, _ = s.DB.Exec(`DETACH DATABASE restored`) }()
	var check string
	if err = s.DB.QueryRow(`PRAGMA restored.integrity_check`).Scan(&check); err != nil || check != "ok" {
		return errors.New("backup integrity check failed")
	}
	var tables int
	if err = s.DB.QueryRow(`SELECT count(*) FROM restored.sqlite_master WHERE type='table' AND name IN ('configs','meta','events')`).Scan(&tables); err != nil || tables != 3 {
		return errors.New("not an SBRoute backup")
	}
	var body []byte
	var revision int64
	if err = s.DB.QueryRow(`SELECT version,body FROM restored.configs WHERE state='active' ORDER BY version DESC LIMIT 1`).Scan(&revision, &body); err != nil {
		return errors.New("backup has no active configuration")
	}
	var restored model.Config
	if err = json.Unmarshal(body, &restored); err != nil {
		return err
	}
	restored.Normalize()
	if err = restored.Validate(); err != nil {
		return fmt.Errorf("invalid backup configuration: %w", err)
	}
	validator, err := core.New(context.Background(), nil)
	if err != nil {
		return err
	}
	err = validator.Validate(restored)
	validator.Close()
	if err != nil {
		return fmt.Errorf("backup incompatible with embedded core: %w", err)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"configs", "meta", "events"} {
		if _, err = tx.Exec(`DELETE FROM main.` + table); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO main.` + table + ` SELECT * FROM restored.` + table); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	c, err := s.Load()
	if err == nil {
		err = c.Validate()
	}
	if err != nil {
		return fmt.Errorf("restored database requires correction; previous backup is %s: %w", backup, err)
	}
	fmt.Println("Restored. Previous state:", backup)
	return nil
}

// parsePrefixes accepts bare addresses as single-host prefixes.
func parsePrefixes(value string) ([]netip.Prefix, error) {
	var v []netip.Prefix
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !strings.Contains(item, "/") {
			addr, err := netip.ParseAddr(item)
			if err != nil {
				return nil, fmt.Errorf("--trusted-proxy: %w", err)
			}
			v = append(v, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(item)
		if err != nil {
			return nil, fmt.Errorf("--trusted-proxy: %w", err)
		}
		v = append(v, p.Masked())
	}
	return v, nil
}
