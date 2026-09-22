package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"codearts2api/internal/auth"
	"codearts2api/internal/pool"
	"codearts2api/internal/scheduler"
	"codearts2api/internal/server"
	"codearts2api/internal/upstream"
)

// 构建期由 -ldflags 注入（见 .goreleaser.yaml 与 Makefile）：
//   -X main.version=v1.2.3 -X main.commit=abc1234 -X main.buildDate=2026-09-21T...
// 未注入时保持 dev，供本地开发与「非 release 构建」区分。
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	cfgPath := flag.String("config", "config.json", "path to config json")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("codearts2api %s (commit %s, built %s)\n", version, commit, buildDate)
		return
	}

	cfg, err := Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	// 应用面板配置页保存的覆盖层（data/settings.json，与 state 文件同目录）。
	cfg.ApplyOverlay(filepath.Join(filepath.Dir(cfg.StateFile), "settings.json"))

	auths, err := auth.LoadDir(cfg.AuthDir)
	if err != nil {
		log.Fatalf("load auths: %v", err)
	}
	log.Printf("loaded %d account(s) from %s", len(auths), cfg.AuthDir)
	if len(auths) == 0 {
		log.Printf("WARNING: no accounts found. Run `codearts2api login` first.")
	}

	if cfg.StateFile != "" {
		_ = os.MkdirAll(filepath.Dir(cfg.StateFile), 0o700)
	}
	p, err := pool.New(auths, cfg.ToPoolConfig(), cfg.StateFile)
	if err != nil {
		log.Fatalf("build pool: %v", err)
	}

	c := upstream.New(120 * time.Second)
	c.SetBenefitAutoClaim(cfg.BenefitAutoClaim)

	sch := scheduler.New(scheduler.Config{
		Pool:              p,
		Enabled:           cfg.Watch.Enabled,
		PollInterval:      time.Duration(cfg.Watch.PollMinutes) * time.Minute,
		RefreshSkew:       time.Duration(cfg.Watch.RefreshSkewM) * time.Minute,
		KeepaliveInterval: time.Duration(cfg.Watch.KeepaliveInterval) * time.Minute,
	})

	h := server.NewHandler(server.Config{
		Pool:         p,
		Upstream:     c,
		APIKey:       cfg.APIKey,
		MaxRotate:    3,
		SoftCooldown: cfg.SoftRateDur,
		ErrThreshold: cfg.Cooldown.ErrThresh,
		ErrCooldown:  cfg.ErrCooldownDur,
		DefaultModel: cfg.DefaultModel,
		QueueRetryDelay: func() time.Duration {
			if cfg.QueueRetrySeconds > 0 {
				return time.Duration(cfg.QueueRetrySeconds) * time.Second
			}
			return 10 * time.Second
		}(),
		QueueMaxAttempts: cfg.QueueMaxAttempts,
		ConvStateFile:    cfg.StateFile + ".chats.json",
		ModelCacheFile:   cfg.StateFile + ".models.json",
		WatchInfo: map[string]any{
			"enabled":              cfg.Watch.Enabled,
			"poll_minutes":         cfg.Watch.PollMinutes,
			"refresh_skew_minutes": cfg.Watch.RefreshSkewM,
			"keepalive_interval":   cfg.Watch.KeepaliveInterval,
			"max_concurrent":       cfg.MaxConcurrent,
			"keepalive_window":     "10m",
		},
		AuthDir:           cfg.AuthDir,
		Listen:            cfg.Listen,
		Version:           version,
		UpdateRepo:        cfg.UpdateRepo,
		UpdateProxy:       cfg.UpdateProxy,
		OAuthClient:       upstream.New(15 * time.Second),
		LoginConfig:       loginConfig(cfg),
		OAuthCallbackHost: cfg.OAuthCallbackHost,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go sch.Run(ctx)
	h.StartAvailabilityProber(ctx)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           h,
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("codearts2api %s listening on %s (commit=%s api_key=%v)", version, cfg.Listen, commit, cfg.APIKey != "")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http: %v", err)
	}
	log.Printf("bye")
}

// loginConfig 决定 WebUI 登录使用的 OAuth 配置。
//
// client_id 优先取已有账号记录的取值（refresh_token 与 client_id 绑定，混用会让
// 老账号刷新失败），其次取配置项，最后才是内置默认值。其余字段必须来自
// DefaultLoginConfig（否则 authorize 链接会缺 host / plugin 信息）。
func loginConfig(cfg *Config) upstream.LoginConfig {
	lc := upstream.DefaultLoginConfig()
	if cfg.LoginClientID != "" {
		lc.ClientID = cfg.LoginClientID
	}
	if auths, err := auth.LoadDir(cfg.AuthDir); err == nil {
		for _, a := range auths {
			if id := a.ClientIDOr(""); id != "" {
				lc.ClientID = id
				break
			}
		}
	}
	return lc
}
