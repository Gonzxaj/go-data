package app

import (
	"context"
	"fmt"
	"go-data/internal/alarm"
	"go-data/internal/api"
	"go-data/internal/auth"
	"go-data/internal/collector"
	"go-data/internal/config"
	"go-data/internal/netmon"
	"go-data/internal/security"
	"go-data/internal/storage"
	"go-data/internal/uptime"
	"log"
	"net/http"
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
	mux.Handle("/", http.FileServer(http.Dir("web/static")))

	var handler http.Handler = mux
	if cfg.AuthEnabled {
		if cfg.AuthUser == "" || cfg.AuthPassword == "" {
			log.Fatal("AUTH_ENABLED requires AUTH_USER and AUTH_PASSWORD to be set")
		}
		handler = auth.BasicAuth(cfg.AuthUser, cfg.AuthPassword, mux)
	}

	http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), handler)
}
