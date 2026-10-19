package app

import (
	"context"
	"go-data/internal/alarm"
	"go-data/internal/api"
	"go-data/internal/auth"
	"go-data/internal/collector"
	"go-data/internal/config"
	"go-data/internal/netmon"
	"go-data/internal/security"
	"go-data/internal/storage"
	"go-data/internal/uptime"
	"go-data/internal/users"
	"go-data/internal/web"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func Build(cfg config.Config) {
	mem := storage.NewMemoryStore()

	var stores []storage.Storage
	stores = append(stores, mem)

	var inf *storage.InfluxStore
	if cfg.UseInflux {
		inf = storage.NewInfluxStore(cfg.InfluxURL, cfg.Token, cfg.Org, cfg.Bucket)
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

	var netMonitor *netmon.Monitor
	if cfg.NetMonEnabled {
		// inf may be a nil *storage.InfluxStore; wrapping that directly in
		// the netmon.PointWriter interface would make it a non-nil
		// interface holding a nil pointer, which netmon's own "== nil"
		// check can't see through — so only assign the interface when inf
		// is genuinely non-nil.
		var pw netmon.PointWriter
		if inf != nil {
			pw = inf
		}
		netMonitor = netmon.New(netmon.Config{
			ProcPath:      cfg.HostProcPath,
			DataDir:       cfg.DataDir,
			ForceFallback: cfg.NetMonForceFallback,
			IPHistoryCap:  cfg.NetMonIPHistoryCap,

			IncludeOutbound: cfg.NetMonOutbound,
		}, pw)
		interval := 10 * time.Second
		if cfg.NetMonIntervalSec > 0 {
			interval = time.Duration(cfg.NetMonIntervalSec) * time.Second
		}
		go netMonitor.Run(context.Background(), interval)
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
		NetMon: netMonitor,
	}
	mux := http.NewServeMux()
	h.Register(mux)
	if err := web.Register(mux, "web/templates", host); err != nil {
		log.Fatalf("render pages: %v", err)
	}
	mux.Handle("/", http.FileServer(noDirListing{http.Dir("web/static")}))

	var handler http.Handler = mux
	switch cfg.AuthMode {
	case "none":
		log.Print("WARNING: AUTH_MODE=none — host data is open to anyone who can reach the port")
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

	// timeouts: without them a slowloris client holds connections forever
	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.ListenAddr, strconv.Itoa(cfg.Port)),
		Handler:           secureHeaders(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
	log.Fatal(srv.ListenAndServe())
}

// secureHeaders blocks framing (clickjacking), MIME sniffing and, via CSP,
// any script not served by us. style 'unsafe-inline' is for the templates'
// style="" attributes; it doesn't allow script execution.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// noDirListing makes the static file server 404 on directories instead of
// listing their contents.
type noDirListing struct{ http.FileSystem }

func (fs noDirListing) Open(name string) (http.File, error) {
	f, err := fs.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err != nil || st.IsDir() {
		f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
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
