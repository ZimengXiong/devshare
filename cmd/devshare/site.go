package main

import (
	"context"
	"github.com/hashicorp/yamux"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"time"
)

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
