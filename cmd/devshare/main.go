package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"golang.org/x/oauth2"
	_ "modernc.org/sqlite"
)

//go:embed web/*
var web embed.FS

const version = "0.1.0"

type Config struct {
	Listen, PublicURL, SiteDomain, DataDir, BootstrapToken string
	DefaultTTL, MaxTTL                                     time.Duration
	OIDCIssuer, OIDCClientID, OIDCClientSecret             string
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
	ID, Hostname, Kind, Visibility, OwnerTokenID, TunnelSecret string
	ExpiresAt                                                  sql.NullTime
	CreatedAt                                                  time.Time
}

type tunnelConn struct {
	*websocket.Conn
	r  io.Reader
	mu sync.Mutex
}

func (c *tunnelConn) Read(p []byte) (int, error) {
	for {
		if c.r != nil {
			n, e := c.r.Read(p)
			if e == io.EOF {
				c.r = nil
				if n > 0 {
					return n, nil
				}
				continue
			}
			return n, e
		}
		mt, r, e := c.NextReader()
		if e != nil {
			return 0, e
		}
		if mt == websocket.BinaryMessage {
			c.r = r
		}
	}
}
func (c *tunnelConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.WriteMessage(websocket.BinaryMessage, p)
	if e != nil {
		return 0, e
	}
	return len(p), nil
}
func (c *tunnelConn) LocalAddr() net.Addr                { return dummyAddr("local") }
func (c *tunnelConn) RemoteAddr() net.Addr               { return dummyAddr("remote") }
func (c *tunnelConn) SetDeadline(t time.Time) error      { return nil }
func (c *tunnelConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *tunnelConn) SetWriteDeadline(t time.Time) error { return nil }

type dummyAddr string

func (d dummyAddr) Network() string { return "websocket" }
func (d dummyAddr) String() string  { return string(d) }

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func loadConfig() Config {
	return Config{
		Listen: env("DEVSHARE_LISTEN", ":8080"), PublicURL: strings.TrimRight(env("DEVSHARE_PUBLIC_URL", "http://localhost:8080"), "/"),
		SiteDomain: env("DEVSHARE_SITE_DOMAIN", "localhost"), DataDir: env("DEVSHARE_DATA_DIR", "./data"), BootstrapToken: os.Getenv("DEVSHARE_BOOTSTRAP_TOKEN"),
		DefaultTTL: durationEnv("DEVSHARE_DEFAULT_TTL", "24h"), MaxTTL: durationEnv("DEVSHARE_MAX_TTL", "168h"),
		OIDCIssuer: os.Getenv("DEVSHARE_OIDC_ISSUER"), OIDCClientID: os.Getenv("DEVSHARE_OIDC_CLIENT_ID"), OIDCClientSecret: os.Getenv("DEVSHARE_OIDC_CLIENT_SECRET"),
	}
}
func durationEnv(k, d string) time.Duration {
	v, e := time.ParseDuration(env(k, d))
	if e != nil {
		log.Fatalf("%s: %v", k, e)
	}
	return v
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "server":
		runServer()
	case "publish":
		publish()
	case "serve":
		serve()
	case "list", "ls":
		listShares()
	case "remove", "rm":
		removeShare()
	case "auth":
		auth()
	case "token":
		tokenCommand()
	case "version", "--version":
		fmt.Println("devshare", version)
	default:
		usage()
		os.Exit(2)
	}
}
func usage() {
	fmt.Print(`devshare — publish a page or share a local server

  devshare auth login --url https://share.example.com --token ds_...
  devshare publish ./dist [--public] [--keep|--ttl 2h]
  devshare serve 5173 [--public] [--ttl 2h]
  devshare list
  devshare rm <share-id>
  devshare server
`)
}

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
		s.verifier = p.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
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
func (s *Server) migrate() error {
	_, e := s.db.Exec(`
CREATE TABLE IF NOT EXISTS tokens(id TEXT PRIMARY KEY, hash TEXT UNIQUE NOT NULL, label TEXT NOT NULL, scopes TEXT NOT NULL, created_at DATETIME NOT NULL, revoked_at DATETIME);
CREATE TABLE IF NOT EXISTS shares(id TEXT PRIMARY KEY, hostname TEXT UNIQUE NOT NULL, kind TEXT NOT NULL, visibility TEXT NOT NULL, owner_token_id TEXT NOT NULL, tunnel_secret TEXT, expires_at DATETIME, created_at DATETIME NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY, email TEXT NOT NULL, expires_at DATETIME NOT NULL);
CREATE TABLE IF NOT EXISTS handoffs(code_hash TEXT PRIMARY KEY, email TEXT NOT NULL, hostname TEXT NOT NULL, return_path TEXT NOT NULL, expires_at DATETIME NOT NULL);
`)
	return e
}
func hash(v string) string { x := sha256.Sum256([]byte(v)); return hex.EncodeToString(x[:]) }
func (s *Server) ensureBootstrap(tok string) error {
	_, e := s.db.Exec(`INSERT OR IGNORE INTO tokens(id,hash,label,scopes,created_at) VALUES(?,?,?,?,?)`, "tok_bootstrap", hash(tok), "bootstrap", "publish,public,keep,tunnel,list,delete,admin", time.Now().UTC())
	return e
}
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}
func (s *Server) authorize(r *http.Request, scope string) (string, bool) {
	tok := bearer(r)
	if tok == "" {
		return "", false
	}
	var id, scopes string
	err := s.db.QueryRow(`SELECT id,scopes FROM tokens WHERE hash=? AND revoked_at IS NULL`, hash(tok)).Scan(&id, &scopes)
	if err != nil {
		return "", false
	}
	for _, v := range strings.Split(scopes, ",") {
		if v == scope || v == "admin" {
			return id, true
		}
	}
	return "", false
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func hostOnly(h string) string { h = strings.ToLower(strings.Split(h, ":")[0]); return h }

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	h := hostOnly(r.Host)
	control, _ := url.Parse(s.cfg.PublicURL)
	if h == hostOnly(control.Host) || (s.cfg.SiteDomain == "localhost" && h == "localhost") {
		s.control(w, r)
		return
	}
	s.site(w, r, h)
}
func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case p == "/" && r.Method == "GET":
		b, _ := web.ReadFile("web/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	case p == "/healthz":
		writeJSON(w, 200, map[string]string{"status": "ok", "version": version})
	case p == "/v1/shares" && r.Method == "POST":
		s.createShare(w, r)
	case p == "/v1/shares" && r.Method == "GET":
		s.list(w, r)
	case p == "/v1/tokens" && r.Method == "POST":
		s.createToken(w, r)
	case strings.HasPrefix(p, "/v1/shares/") && strings.HasSuffix(p, "/content") && r.Method == "PUT":
		s.upload(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/v1/shares/"), "/content"))
	case strings.HasPrefix(p, "/v1/shares/") && r.Method == "DELETE":
		s.remove(w, r, strings.TrimPrefix(p, "/v1/shares/"))
	case strings.HasPrefix(p, "/v1/tunnels/") && strings.HasSuffix(p, "/connect"):
		s.connectTunnel(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/v1/tunnels/"), "/connect"))
	case p == "/auth/login":
		s.login(w, r)
	case p == "/auth/callback":
		s.callback(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(r, "admin"); !ok {
		http.Error(w, "admin scope required", http.StatusForbidden)
		return
	}
	var q struct {
		Label  string   `json:"label"`
		Scopes []string `json:"scopes"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q) != nil || strings.TrimSpace(q.Label) == "" || len(q.Scopes) == 0 {
		http.Error(w, "label and scopes are required", http.StatusBadRequest)
		return
	}
	allowed := map[string]bool{"publish": true, "public": true, "keep": true, "tunnel": true, "list": true, "delete": true, "admin": true}
	for _, scope := range q.Scopes {
		if !allowed[scope] {
			http.Error(w, "unknown scope: "+scope, http.StatusBadRequest)
			return
		}
	}
	token := "ds_" + randomText(40)
	id := "tok_" + randomText(12)
	_, err := s.db.Exec(`INSERT INTO tokens(id,hash,label,scopes,created_at) VALUES(?,?,?,?,?)`, id, hash(token), strings.TrimSpace(q.Label), strings.Join(q.Scopes, ","), time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "label": q.Label, "scopes": q.Scopes, "token": token})
}

var adjectives = []string{"amber", "brisk", "calm", "clear", "gentle", "lilac", "quiet", "rapid", "silver", "small", "soft", "solar", "vivid", "warm"}
var nouns = []string{"brook", "cedar", "comet", "dawn", "field", "harbor", "lake", "meadow", "orbit", "otter", "panda", "pine", "river", "sparrow"}

func randomText(n int) string {
	const a = "23456789abcdefghjkmnpqrstuvwxyz"
	b := make([]byte, n)
	x := make([]byte, n)
	_, _ = rand.Read(x)
	for i := range b {
		b[i] = a[int(x[i])%len(a)]
	}
	return string(b)
}
func (s *Server) newNames() (string, string) {
	for {
		suffix := randomText(4)
		h := adjectives[int(suffix[0])%len(adjectives)] + "-" + nouns[int(suffix[1])%len(nouns)] + "-" + suffix
		var n int
		if s.db.QueryRow(`SELECT count(*) FROM shares WHERE hostname=?`, h+"."+s.cfg.SiteDomain).Scan(&n) == nil && n == 0 {
			return "shr_" + randomText(12), h + "." + s.cfg.SiteDomain
		}
	}
}
func (s *Server) createShare(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(r, "publish")
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var q struct {
		Kind, Visibility, TTL string `json:",omitempty"`
		Keep                  bool
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q)
	if q.Kind == "" {
		q.Kind = "static"
	}
	if q.Visibility == "" {
		q.Visibility = "private"
	}
	if q.Visibility == "public" {
		if _, ok = s.authorize(r, "public"); !ok {
			http.Error(w, "public scope required", 403)
			return
		}
	}
	if q.Kind == "tunnel" {
		if _, ok = s.authorize(r, "tunnel"); !ok {
			http.Error(w, "tunnel scope required", 403)
			return
		}
	}
	var exp any
	var expires *time.Time
	if !q.Keep {
		d := s.cfg.DefaultTTL
		if q.TTL != "" {
			var e error
			d, e = time.ParseDuration(q.TTL)
			if e != nil || d <= 0 || d > s.cfg.MaxTTL {
				http.Error(w, "invalid ttl", 400)
				return
			}
		}
		t := time.Now().UTC().Add(d)
		expires = &t
		exp = t
	} else if _, ok = s.authorize(r, "keep"); !ok {
		http.Error(w, "keep scope required", 403)
		return
	}
	shareID, hostname := s.newNames()
	secret := ""
	if q.Kind == "tunnel" {
		secret = "dst_" + randomText(32)
	}
	_, e := s.db.Exec(`INSERT INTO shares(id,hostname,kind,visibility,owner_token_id,tunnel_secret,expires_at,created_at) VALUES(?,?,?,?,?,?,?,?)`, shareID, hostname, q.Kind, q.Visibility, id, hash(secret), exp, time.Now().UTC())
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, 201, map[string]any{"id": shareID, "hostname": hostname, "url": "https://" + hostname, "visibility": q.Visibility, "expiresAt": expires, "tunnelSecret": secret})
}
func (s *Server) getShare(id string) (Share, error) {
	var x Share
	err := s.db.QueryRow(`SELECT id,hostname,kind,visibility,owner_token_id,coalesce(tunnel_secret,''),expires_at,created_at FROM shares WHERE id=?`, id).Scan(&x.ID, &x.Hostname, &x.Kind, &x.Visibility, &x.OwnerTokenID, &x.TunnelSecret, &x.ExpiresAt, &x.CreatedAt)
	return x, err
}
func (s *Server) owned(r *http.Request, share Share, scope string) bool {
	id, ok := s.authorize(r, scope)
	return ok && (id == share.OwnerTokenID || func() bool { _, a := s.authorize(r, "admin"); return a }())
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request, id string) {
	x, e := s.getShare(id)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if !s.owned(r, x, "publish") {
		http.Error(w, "forbidden", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	f, _, e := r.FormFile("content")
	if e != nil {
		http.Error(w, "content archive required", 400)
		return
	}
	defer f.Close()
	tmp := filepath.Join(s.cfg.DataDir, "sites", "."+id+"-"+randomText(6))
	if e = extractTarGz(f, tmp); e != nil {
		os.RemoveAll(tmp)
		http.Error(w, e.Error(), 400)
		return
	}
	dest := filepath.Join(s.cfg.DataDir, "sites", id)
	old := dest + ".old"
	_ = os.RemoveAll(old)
	_ = os.Rename(dest, old)
	if e = os.Rename(tmp, dest); e != nil {
		_ = os.Rename(old, dest)
		http.Error(w, e.Error(), 500)
		return
	}
	_ = os.RemoveAll(old)
	writeJSON(w, 200, map[string]string{"id": id, "url": "https://" + x.Hostname})
}
func extractTarGz(src io.Reader, dest string) error {
	gz, e := gzip.NewReader(src)
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count := 0
	var total int64
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		count++
		if count > 5000 {
			return errors.New("too many files")
		}
		name := filepath.Clean(h.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return errors.New("unsafe archive path")
		}
		target := filepath.Join(dest, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if e = os.MkdirAll(target, 0750); e != nil {
				return e
			}
		case tar.TypeReg:
			total += h.Size
			if total > 256<<20 {
				return errors.New("archive expands beyond 256 MiB")
			}
			if e = os.MkdirAll(filepath.Dir(target), 0750); e != nil {
				return e
			}
			f, e := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
			if e != nil {
				return e
			}
			_, e = io.CopyN(f, tr, h.Size)
			ce := f.Close()
			if e != nil {
				return e
			}
			if ce != nil {
				return ce
			}
		default:
			return errors.New("archive contains unsupported entry")
		}
	}
	if _, e = os.Stat(filepath.Join(dest, "index.html")); e != nil {
		return errors.New("archive must contain index.html at its root")
	}
	return nil
}
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	id, ok := s.authorize(r, "list")
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	rows, e := s.db.Query(`SELECT id,hostname,kind,visibility,expires_at,created_at FROM shares WHERE owner_token_id=? ORDER BY created_at DESC`, id)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var i, h, k, v string
		var ex sql.NullTime
		var cr time.Time
		_ = rows.Scan(&i, &h, &k, &v, &ex, &cr)
		out = append(out, map[string]any{"id": i, "url": "https://" + h, "kind": k, "visibility": v, "expiresAt": func() any {
			if ex.Valid {
				return ex.Time
			}
			return nil
		}(), "createdAt": cr})
	}
	writeJSON(w, 200, out)
}
func (s *Server) remove(w http.ResponseWriter, r *http.Request, id string) {
	x, e := s.getShare(id)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if !s.owned(r, x, "delete") {
		http.Error(w, "forbidden", 403)
		return
	}
	_, _ = s.db.Exec(`DELETE FROM shares WHERE id=?`, id)
	_ = os.RemoveAll(filepath.Join(s.cfg.DataDir, "sites", id))
	if v, ok := s.tunnels.LoadAndDelete(id); ok {
		_ = v.(*yamux.Session).Close()
	}
	w.WriteHeader(204)
}

func (s *Server) site(w http.ResponseWriter, r *http.Request, h string) {
	if s.completeHandoff(w, r, h) {
		return
	}
	var x Share
	e := s.db.QueryRow(`SELECT id,hostname,kind,visibility,owner_token_id,coalesce(tunnel_secret,''),expires_at,created_at FROM shares WHERE hostname=? AND (expires_at IS NULL OR expires_at>?)`, h, time.Now().UTC()).Scan(&x.ID, &x.Hostname, &x.Kind, &x.Visibility, &x.OwnerTokenID, &x.TunnelSecret, &x.ExpiresAt, &x.CreatedAt)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if x.Visibility == "private" && !s.viewerOK(r) {
		s.beginLogin(w, r, h)
		return
	}
	if x.Kind == "static" {
		http.FileServer(http.Dir(filepath.Join(s.cfg.DataDir, "sites", x.ID))).ServeHTTP(w, r)
		return
	}
	v, ok := s.tunnels.Load(x.ID)
	if !ok {
		http.Error(w, "This live share is offline.", 503)
		return
	}
	session := v.(*yamux.Session)
	proxy := &httputil.ReverseProxy{Director: func(req *http.Request) { req.URL.Scheme = "http"; req.URL.Host = "devshare-tunnel"; req.Host = h }, Transport: &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) { return session.Open() }}}
	proxy.ServeHTTP(w, r)
}
func (s *Server) connectTunnel(w http.ResponseWriter, r *http.Request, id string) {
	x, e := s.getShare(id)
	if e != nil || x.Kind != "tunnel" || hash(bearer(r)) != x.TunnelSecret {
		http.Error(w, "unauthorized", 401)
		return
	}
	ws, e := s.upgrader.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	conn := &tunnelConn{Conn: ws}
	session, e := yamux.Server(conn, nil)
	if e != nil {
		_ = ws.Close()
		return
	}
	if old, ok := s.tunnels.LoadOrStore(id, session); ok {
		_ = old.(*yamux.Session).Close()
		s.tunnels.Store(id, session)
	}
	log.Printf("tunnel %s connected", id)
	<-session.CloseChan()
	s.tunnels.CompareAndDelete(id, session)
	_ = ws.Close()
}

func (s *Server) viewerOK(r *http.Request) bool {
	c, e := r.Cookie("devshare_session")
	if e != nil {
		return false
	}
	var n int
	e = s.db.QueryRow(`SELECT count(*) FROM sessions WHERE token_hash=? AND expires_at>?`, hash(c.Value), time.Now().UTC()).Scan(&n)
	return e == nil && n == 1
}
func safeReturn(v string) string {
	u, e := url.Parse(v)
	if e != nil || u.Scheme != "https" || u.Host == "" {
		return "/"
	}
	return u.String()
}
func (s *Server) beginLogin(w http.ResponseWriter, r *http.Request, h string) {
	if s.oauth == nil {
		http.Error(w, "private viewing is not configured", 503)
		return
	}
	ret := "https://" + h + r.URL.RequestURI()
	http.Redirect(w, r, s.cfg.PublicURL+"/auth/login?return="+url.QueryEscape(ret), 302)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.oauth == nil {
		http.Error(w, "OIDC not configured", 503)
		return
	}
	state := randomText(32)
	ret := safeReturn(r.URL.Query().Get("return"))
	http.SetCookie(w, &http.Cookie{Name: "devshare_oauth", Value: state + "|" + base64.RawURLEncoding.EncodeToString([]byte(ret)), Path: "/auth", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, s.oauth.AuthCodeURL(state), 302)
}
func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	c, e := r.Cookie("devshare_oauth")
	if e != nil {
		http.Error(w, "login expired", 400)
		return
	}
	parts := strings.SplitN(c.Value, "|", 2)
	if len(parts) != 2 || parts[0] != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", 400)
		return
	}
	tok, e := s.oauth.Exchange(r.Context(), r.URL.Query().Get("code"))
	if e != nil {
		http.Error(w, "login failed", 401)
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idtok, e := s.verifier.Verify(r.Context(), raw)
	if e != nil {
		http.Error(w, "invalid identity", 401)
		return
	}
	var claims struct {
		Email string `json:"email"`
	}
	_ = idtok.Claims(&claims)
	retBytes, _ := base64.RawURLEncoding.DecodeString(parts[1])
	ret := safeReturn(string(retBytes))
	u, _ := url.Parse(ret)
	code := randomText(40)
	_, _ = s.db.Exec(`INSERT INTO handoffs(code_hash,email,hostname,return_path,expires_at) VALUES(?,?,?,?,?)`, hash(code), claims.Email, u.Hostname(), u.RequestURI(), time.Now().UTC().Add(2*time.Minute))
	http.Redirect(w, r, "https://"+u.Hostname()+"/__devshare/session?code="+code, 302)
}
func (s *Server) completeHandoff(w http.ResponseWriter, r *http.Request, h string) bool {
	if r.URL.Path != "/__devshare/session" {
		return false
	}
	code := r.URL.Query().Get("code")
	var email, host, ret string
	e := s.db.QueryRow(`SELECT email,hostname,return_path FROM handoffs WHERE code_hash=? AND expires_at>?`, hash(code), time.Now().UTC()).Scan(&email, &host, &ret)
	if e != nil || host != h {
		http.Error(w, "invalid login handoff", 400)
		return true
	}
	_, _ = s.db.Exec(`DELETE FROM handoffs WHERE code_hash=?`, hash(code))
	session := "dvs_" + randomText(40)
	_, _ = s.db.Exec(`INSERT INTO sessions(token_hash,email,expires_at) VALUES(?,?,?)`, hash(session), email, time.Now().UTC().Add(12*time.Hour))
	http.SetCookie(w, &http.Cookie{Name: "devshare_session", Value: session, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 43200})
	http.Redirect(w, r, ret, 302)
	return true
}
func (s *Server) janitor() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for range t.C {
		rows, _ := s.db.Query(`SELECT id FROM shares WHERE expires_at IS NOT NULL AND expires_at<=?`, time.Now().UTC())
		var ids []string
		for rows != nil && rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		if rows != nil {
			rows.Close()
		}
		for _, id := range ids {
			_, _ = s.db.Exec(`DELETE FROM shares WHERE id=?`, id)
			_ = os.RemoveAll(filepath.Join(s.cfg.DataDir, "sites", id))
		}
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at<=?; DELETE FROM handoffs WHERE expires_at<=?`, time.Now().UTC(), time.Now().UTC())
	}
}

type clientConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func configPath() string {
	d, e := os.UserConfigDir()
	if e != nil {
		d = "."
	}
	return filepath.Join(d, "devshare", "config.json")
}
func client() clientConfig {
	c := clientConfig{URL: os.Getenv("DEVSHARE_URL"), Token: os.Getenv("DEVSHARE_TOKEN")}
	if b, e := os.ReadFile(configPath()); e == nil {
		var f clientConfig
		_ = json.Unmarshal(b, &f)
		if c.URL == "" {
			c.URL = f.URL
		}
		if c.Token == "" {
			c.Token = f.Token
		}
	}
	c.URL = strings.TrimRight(c.URL, "/")
	if c.URL == "" || c.Token == "" {
		log.Fatal("authenticate with `devshare auth login --url ... --token ...` or set DEVSHARE_URL and DEVSHARE_TOKEN")
	}
	return c
}
func auth() {
	if len(os.Args) < 3 || os.Args[2] != "login" {
		log.Fatal("usage: devshare auth login --url URL --token TOKEN")
	}
	fs := flag.NewFlagSet("auth login", flag.ExitOnError)
	u := fs.String("url", "", "API URL")
	t := fs.String("token", "", "API token")
	_ = fs.Parse(os.Args[3:])
	if *u == "" || *t == "" {
		log.Fatal("--url and --token are required")
	}
	p := configPath()
	_ = os.MkdirAll(filepath.Dir(p), 0700)
	b, _ := json.MarshalIndent(clientConfig{URL: strings.TrimRight(*u, "/"), Token: *t}, "", "  ")
	if e := os.WriteFile(p, b, 0600); e != nil {
		log.Fatal(e)
	}
	fmt.Println("Authenticated with", *u)
}

func tokenCommand() {
	if len(os.Args) < 3 || os.Args[2] != "create" {
		log.Fatal("usage: devshare token create --label NAME --scopes publish,list,delete")
	}
	fs := flag.NewFlagSet("token create", flag.ExitOnError)
	label := fs.String("label", "", "token label")
	scopes := fs.String("scopes", "publish,list,delete", "comma-separated scopes")
	_ = fs.Parse(os.Args[3:])
	if strings.TrimSpace(*label) == "" {
		log.Fatal("--label is required")
	}
	c := client()
	body, _ := json.Marshal(map[string]any{"label": *label, "scopes": strings.Split(*scopes, ",")})
	req, _ := http.NewRequest("POST", c.URL+"/v1/tokens", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("server: %s", b)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	fmt.Println(out["token"])
	fmt.Println("Save this token now; it will not be shown again.")
}
func parseShareFlags(name string, args []string) (*flag.FlagSet, *bool, *bool, *string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	pub := fs.Bool("public", false, "allow anyone to view")
	keep := fs.Bool("keep", false, "never expire")
	ttl := fs.String("ttl", "", "expiration such as 2h")
	return fs, pub, keep, ttl
}
func createRemote(c clientConfig, kind string, pub, keep bool, ttl string) map[string]any {
	body, _ := json.Marshal(map[string]any{"kind": kind, "visibility": map[bool]string{true: "public", false: "private"}[pub], "keep": keep, "ttl": ttl})
	req, _ := http.NewRequest("POST", c.URL+"/v1/shares", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("server: %s: %s", resp.Status, string(b))
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}
func publish() {
	fs, pub, keep, ttl := parseShareFlags("publish", os.Args[2:])
	_ = fs.Parse(os.Args[2:])
	if fs.NArg() != 1 {
		log.Fatal("usage: devshare publish <file-or-directory> [--public] [--keep|--ttl 2h]")
	}
	c := client()
	out := createRemote(c, "static", *pub, *keep, *ttl)
	id := out["id"].(string)
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, e := mw.CreateFormFile("content", "site.tar.gz")
		if e == nil {
			e = pack(fs.Arg(0), part)
		}
		_ = mw.Close()
		_ = pw.CloseWithError(e)
	}()
	req, _ := http.NewRequest("PUT", c.URL+"/v1/shares/"+id+"/content", pr)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("upload: %s", b)
	}
	fmt.Println(out["url"])
	fmt.Printf("%s · %s\n", map[bool]string{true: "public", false: "private"}[*pub], map[bool]string{true: "no expiration", false: "temporary"}[*keep])
}
func pack(path string, w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	info, e := os.Stat(path)
	if e != nil {
		return e
	}
	root := path
	if !info.IsDir() {
		root = filepath.Dir(path)
	}
	e = filepath.Walk(root, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if i.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported: %s", p)
		}
		h, e := tar.FileInfoHeader(i, "")
		if e != nil {
			return e
		}
		h.Name = filepath.ToSlash(rel)
		if e = tw.WriteHeader(h); e != nil {
			return e
		}
		if i.Mode().IsRegular() {
			f, e := os.Open(p)
			if e != nil {
				return e
			}
			_, e = io.Copy(tw, f)
			_ = f.Close()
			return e
		}
		return nil
	})
	if ce := tw.Close(); e == nil {
		e = ce
	}
	if ce := gz.Close(); e == nil {
		e = ce
	}
	return e
}
func listShares() {
	c := client()
	req, _ := http.NewRequest("GET", c.URL+"/v1/shares", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Fatal(resp.Status)
	}
	var rows []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&rows)
	for _, x := range rows {
		fmt.Printf("%-18s %-8s %-8s %s\n", x["id"], x["kind"], x["visibility"], x["url"])
	}
}
func removeShare() {
	if len(os.Args) != 3 {
		log.Fatal("usage: devshare rm <share-id>")
	}
	c := client()
	req, _ := http.NewRequest("DELETE", c.URL+"/v1/shares/"+os.Args[2], nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Fatal(resp.Status)
	}
	fmt.Println("removed", os.Args[2])
}
func serve() {
	fs, pub, keep, ttl := parseShareFlags("serve", os.Args[2:])
	_ = fs.Parse(os.Args[2:])
	if fs.NArg() != 1 {
		log.Fatal("usage: devshare serve <port> [--public] [--ttl 2h]")
	}
	port, e := strconv.Atoi(fs.Arg(0))
	if e != nil || port < 1 || port > 65535 {
		log.Fatal("invalid port")
	}
	c := client()
	out := createRemote(c, "tunnel", *pub, *keep, *ttl)
	fmt.Println(out["url"])
	wsURL := strings.Replace(c.URL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1) + "/v1/tunnels/" + out["id"].(string) + "/connect"
	headers := http.Header{"Authorization": []string{"Bearer " + out["tunnelSecret"].(string)}}
	for {
		ws, _, e := websocket.DefaultDialer.Dial(wsURL, headers)
		if e != nil {
			log.Printf("connect: %v; retrying", e)
			time.Sleep(2 * time.Second)
			continue
		}
		session, e := yamux.Client(&tunnelConn{Conn: ws}, nil)
		if e != nil {
			_ = ws.Close()
			continue
		}
		for {
			stream, e := session.Accept()
			if e != nil {
				break
			}
			go func(conn net.Conn) {
				up, e := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
				if e != nil {
					_ = conn.Close()
					return
				}
				go func() { _, _ = io.Copy(up, conn); _ = up.(*net.TCPConn).CloseWrite() }()
				_, _ = io.Copy(conn, up)
				_ = conn.Close()
				_ = up.Close()
			}(stream)
		}
		_ = session.Close()
		_ = ws.Close()
		time.Sleep(time.Second)
	}
}

var _ = sort.Strings
var _ = exec.Command
var _ = runtime.GOOS
