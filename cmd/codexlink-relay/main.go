package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shuaigehuang/codexlink/internal/relay"

	"golang.org/x/crypto/acme/autocert"
)

const (
	defaultListen = "127.0.0.1:8787"
	minTokenBytes = 32
)

func main() {
	cfg, listen, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}

	service, err := relay.NewServer(cfg)
	if err != nil {
		log.Fatalf("启动中继失败: %v", err)
	}
	defer service.Close()

	tlsOptions := loadTLSOptions(os.Getenv)
	var certManager *autocert.Manager
	var httpServer *http.Server
	if tlsOptions.Domain != "" {
		if strings.TrimSpace(os.Getenv("CODEXLINK_RELAY_LISTEN")) == "" {
			listen = ":443"
		}
		manager, cfgTLS := newAutocertManager(tlsOptions, cfg.DataDir)
		certManager = manager
		httpServer = &http.Server{
			Addr:              listen,
			Handler:           service.Handler(),
			TLSConfig:         cfgTLS,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
	} else {
		httpServer = &http.Server{
			Addr:              listen,
			Handler:           service.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
	}
	runServer(service, httpServer, certManager, tlsOptions, cfg, listen)
}

func runServer(service *relay.Server, httpServer *http.Server, certManager *autocert.Manager, tlsOptions tlsOptions, cfg relay.Config, listen string) {
	var redirectServer *http.Server
	if certManager != nil && tlsOptions.HTTPAddr != "" {
		redirectServer = &http.Server{
			Addr:              tlsOptions.HTTPAddr,
			Handler:           redirectToHTTPS(certManager, tlsOptions.Domain),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       30 * time.Second,
		}
		serveHTTPRedirect(redirectServer, cfg.Logger)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		if redirectServer != nil {
			_ = redirectServer.Shutdown(shutdownCtx)
		}
		service.Close()
		close(shutdownDone)
	}()

	if certManager != nil {
		cfg.Logger.Printf("relay listening with automatic HTTPS on %s domain=%s data=%s", listen, tlsOptions.Domain, cfg.DataDir)
	} else {
		cfg.Logger.Printf("relay listening on %s data=%s", listen, cfg.DataDir)
	}
	var err error
	if certManager != nil {
		err = httpServer.ListenAndServeTLS("", "")
	} else {
		err = httpServer.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("中继服务异常退出: %v", err)
	}
	if ctx.Err() != nil {
		<-shutdownDone
	}
	cfg.Logger.Print("relay stopped")
}

func loadConfig(getenv func(string) string) (relay.Config, string, error) {
	token := strings.TrimSpace(getenv("CODEXLINK_RELAY_DESKTOP_TOKEN"))
	if len(token) < minTokenBytes {
		return relay.Config{}, "", fmt.Errorf("CODEXLINK_RELAY_DESKTOP_TOKEN 至少需要 %d 个字符", minTokenBytes)
	}
	dataDir := strings.TrimSpace(getenv("CODEXLINK_RELAY_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	listen := strings.TrimSpace(getenv("CODEXLINK_RELAY_LISTEN"))
	if listen == "" {
		listen = defaultListen
	}

	pairTTL, err := durationEnv(getenv, "CODEXLINK_RELAY_PAIR_TTL", 10*time.Minute)
	if err != nil {
		return relay.Config{}, "", err
	}
	sessionTTL, err := durationEnv(getenv, "CODEXLINK_RELAY_SESSION_TTL", 90*24*time.Hour)
	if err != nil {
		return relay.Config{}, "", err
	}
	trustProxy, err := boolEnv(getenv, "CODEXLINK_RELAY_TRUST_PROXY", false)
	if err != nil {
		return relay.Config{}, "", err
	}
	origins := splitList(getenv("CODEXLINK_RELAY_ORIGINS"))

	return relay.Config{
		DesktopToken:   token,
		DataDir:        dataDir,
		PairTTL:        pairTTL,
		SessionTTL:     sessionTTL,
		AllowedOrigins: origins,
		TrustProxy:     trustProxy,
		Logger:         log.New(os.Stderr, "relay ", log.LstdFlags|log.LUTC),
	}, listen, nil
}

func durationEnv(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s 需为有效时长，例如 10m 或 720h", key)
	}
	return parsed, nil
}

func boolEnv(getenv func(string) string, key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s 需为 true/false 或 1/0", key)
	}
	return parsed, nil
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}
