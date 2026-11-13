package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

type tlsOptions struct {
	Domain   string
	Email    string
	HTTPAddr string
}

func loadTLSOptions(getenv func(string) string) tlsOptions {
	options := tlsOptions{
		Domain:   strings.ToLower(strings.TrimSpace(getenv("CODEXLINK_RELAY_TLS_DOMAIN"))),
		Email:    strings.TrimSpace(getenv("CODEXLINK_RELAY_TLS_EMAIL")),
		HTTPAddr: strings.TrimSpace(getenv("CODEXLINK_RELAY_HTTP_ADDR")),
	}
	if options.Domain != "" && options.HTTPAddr == "" {
		options.HTTPAddr = ":80"
	}
	return options
}

func newAutocertManager(options tlsOptions, dataDir string) (*autocert.Manager, *tls.Config) {
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(options.Domain),
		Cache:      autocert.DirCache(filepath.Join(dataDir, "autocert")),
		Email:      options.Email,
	}
	config := manager.TLSConfig()
	config.MinVersion = tls.VersionTLS12
	config.NextProtos = []string{"http/1.1", acme.ALPNProto}
	return manager, config
}

func redirectToHTTPS(manager *autocert.Manager, domain string) http.Handler {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + domain + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
	return manager.HTTPHandler(fallback)
}

func serveHTTPRedirect(server *http.Server, logger *log.Logger) {
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Printf("HTTP redirect listener unavailable on %s: %v", server.Addr, err)
		}
	}()
}
