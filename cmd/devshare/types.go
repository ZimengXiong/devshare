package main

import (
	"database/sql"
	"embed"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/websocket"
	"golang.org/x/oauth2"
	"sync"
	"time"
)

//go:embed web/*
var web embed.FS

type Config struct {
	Listen, PublicURL, SiteDomain, DataDir, BootstrapToken string
	DefaultTTL, MaxTTL                                     time.Duration
	OIDCIssuer, OIDCClientID, OIDCClientSecret             string
	DisableViewerAuth                                      bool
}

type Server struct {
	cfg      Config
	db       *sql.DB
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
	tunnels  sync.Map
	upgrader websocket.Upgrader
}

type Share struct {
	ID, Hostname, Kind, Format, Visibility, OwnerTokenID, TunnelSecret string
	ExpiresAt                                                          sql.NullTime
	CreatedAt                                                          time.Time
}
