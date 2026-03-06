package main

import (
	"net/http"
	"net/url"
	"time"
)

func (s *Server) viewerOK(r *http.Request) bool {
	if s.cfg.DisableViewerAuth {
		return true
	}
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
