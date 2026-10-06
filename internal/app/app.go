package app

import (
	"context"
	"go-data/internal/alarm"
	"go-data/internal/api"
	"go-data/internal/auth"
	"go-data/internal/collector"
	"go-data/internal/config"
	"go-data/internal/security"
	"go-data/internal/storage"
	"go-data/internal/uptime"
	"go-data/internal/users"
	"log"
	"net/http"
	"path/filepath"
	"time"
)

func Build(cfg config.Config) {
	mem := storage.NewMemoryStore()

	var stores []storage.Storage
	stores = append(stores, mem)

	if cfg.UseInflux {
		inf := storage.NewInfluxStore(cfg.InfluxURL, cfg.Token, cfg.Org, cfg.Bucket)
		stores = append(stores, inf)
	}

	store := storage.NewMulti(stores...)

	sysCollector := collector.NewSystemCollector(cfg)
	procCollector := collector.NewProcessCollector(cfg.HostProcPath)
	uptimeLog := uptime.NewLog(cfg.DataDir)
	alarmEval := alarm.NewEvaluator(alarm.DefaultThresholds())
	host := collector.CollectHostInfo(cfg.HostProcPath, cfg.HostRootPath)

	var dockerCollector *collector.DockerCollector
	if cfg.DockerEnabled {
		if dc, err := collector.NewDockerCollector(); err == nil {
			dockerCollector = dc
			go dockerCollector.Run(context.Background(), 2*time.Second)
		} else {
			log.Printf("docker collector disabled: %v", err)
		}
	}

	var sshWatcher *security.SSHWatcher
	if cfg.SSHWatchEnabled {
		sshWatcher = security.NewSSHWatcher(cfg.SSHLogPath, cfg.DataDir)
		go sshWatcher.Run(context.Background(), 2*time.Second)
	}

	var connWatcher *security.ConnWatcher
	if cfg.ConnWatchEnabled && (cfg.ConnWatchAuto || len(cfg.ConnWatchPorts) > 0) {
		connWatcher = security.NewConnWatcher(cfg.ConnWatchPorts, cfg.ConnWatchAuto, cfg.HostProcPath, cfg.DataDir)
		go connWatcher.Run(context.Background(), 5*time.Second)
	}

	go func() {
		for {
			now := time.Now()
			m := sysCollector.Collect()
			m.Processes = procCollector.Top(6)
			store.Save(m)
			uptimeLog.Observe(now, uptime.ReadBootTime(now))
			alarmEval.Evaluate(m)

			time.Sleep(time.Second)
		}
	}()

	h := &api.Handler{
		Mem:    mem,
		Host:   host,
		Docker: dockerCollector,
		Alarms: alarmEval,
		Uptime: uptimeLog,
		SSH:    sshWatcher,
		Conn:   connWatcher,
	}
	mux := http.NewServeMux()
	h.Register(mux)
	mux.Handle("/", http.FileServer(http.Dir("web/static")))

	var handler http.Handler = mux
	switch cfg.AuthMode {
	case "none":
		mux.HandleFunc("GET /api/auth/me", auth.ModeInfo("none"))
	case "basic":
		if cfg.AuthUser == "" || cfg.AuthPassword == "" {
			log.Fatal("AUTH_MODE=basic requires AUTH_USER and AUTH_PASSWORD to be set")
		}
		mux.HandleFunc("GET /api/auth/me", auth.ModeInfo("basic"))
		handler = auth.BasicAuth(cfg.AuthUser, cfg.AuthPassword, mux)
	case "session":
		store := openUserStore(cfg)
		sess := auth.NewSession(store)
		sess.Register(mux)
		(&api.UsersHandler{Store: store}).Register(mux)
		handler = sess.Middleware(mux)
	default:
		log.Fatalf("unknown AUTH_MODE %q (use none, basic or session)", cfg.AuthMode)
	}
	log.Printf("auth mode: %s", cfg.AuthMode)

	http.ListenAndServe(":9000", handler)
}

// openUserStore opens DataDir/users.db and, on first run, seeds an admin from
// AUTH_USER/AUTH_PASSWORD. Afterwards those env vars are ignored: accounts
// are managed from the dashboard.
func openUserStore(cfg config.Config) *users.Store {
	store, err := users.Open(filepath.Join(cfg.DataDir, "users.db"))
	if err != nil {
		log.Fatalf("open user store: %v", err)
	}
	n, err := store.Count()
	if err != nil {
		log.Fatalf("count users: %v", err)
	}
	if n > 0 {
		return store
	}
	if cfg.AuthUser == "" || cfg.AuthPassword == "" {
		log.Fatal("AUTH_MODE=session with no users yet requires AUTH_USER and AUTH_PASSWORD to create the first admin")
	}
	if _, err := store.Create(cfg.AuthUser, cfg.AuthPassword, users.RoleAdmin); err != nil {
		log.Fatalf("create initial admin %q: %v", cfg.AuthUser, err)
	}
	log.Printf("created initial admin %q", cfg.AuthUser)
	return store
}
