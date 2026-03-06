package main

import (
	"context"
	"crypto"
	"database/sql"
	"errors"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/websocket"
	"golang.org/x/oauth2"
	"log"
	_ "modernc.org/sqlite"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func runServer() {
	cfg := loadConfig()
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "sites"), 0750); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(cfg.DataDir, "devshare.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		log.Fatal(err)
	}
	s := &Server{cfg: cfg, db: db, upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
	if err = s.migrate(); err != nil {
		log.Fatal(err)
	}
	if err = s.backfillFormats(); err != nil {
		log.Fatal(err)
	}
	if cfg.BootstrapToken != "" {
		if err = s.ensureBootstrap(cfg.BootstrapToken); err != nil {
			log.Fatal(err)
		}
	}
	if cfg.OIDCIssuer != "" {
		p, e := oidc.NewProvider(context.Background(), cfg.OIDCIssuer)
		if e != nil {
			log.Fatal(e)
		}
		issuer := strings.TrimRight(cfg.OIDCIssuer, "/")
		metadata, metadataErr := loadOIDCMetadata(issuer)
		if metadataErr != nil {
			e = metadataErr
			log.Fatal(e)
		}
		keySet := fallbackKeySet{
			primary:  oidc.NewRemoteKeySet(context.Background(), metadata.JWKS),
			fallback: &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{[]byte(cfg.OIDCClientSecret)}},
		}
		s.verifier = oidc.NewVerifier(metadata.Issuer, keySet, &oidc.Config{ClientID: cfg.OIDCClientID, SupportedSigningAlgs: metadata.Algorithms})
		s.oauth = &oauth2.Config{ClientID: cfg.OIDCClientID, ClientSecret: cfg.OIDCClientSecret, Endpoint: p.Endpoint(), RedirectURL: cfg.PublicURL + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	srv := &http.Server{Addr: cfg.Listen, Handler: mux, ReadHeaderTimeout: 15 * time.Second}
	go s.janitor()
	go func() {
		log.Printf("devshare %s listening on %s", version, cfg.Listen)
		if e := srv.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Fatal(e)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
