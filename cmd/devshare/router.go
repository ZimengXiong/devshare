package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func hostOnly(h string) string { h = strings.ToLower(strings.Split(h, ":")[0]); return h }

func (s *Server) shareURL(host string) string {
	control, err := url.Parse(s.cfg.PublicURL)
	if err != nil || control.Scheme == "" {
		return "https://" + host
	}
	if port := control.Port(); port != "" {
		host += ":" + port
	}
	return control.Scheme + "://" + host
}

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
	control, _ := url.Parse(s.cfg.PublicURL)
	if hostOnly(control.Host) == hostOnly(r.Host) && s.completeHandoff(w, r, hostOnly(r.Host)) {
		return
	}
	switch {
	case p == "/" && r.Method == "GET" && !s.viewerOK(r):
		s.beginLogin(w, r, hostOnly(r.Host))
	case p == "/" && r.Method == "GET":
		b, _ := web.ReadFile("web/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	case p == "/style.css" && r.Method == "GET":
		b, _ := web.ReadFile("web/style.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(b)
	case p == "/rows.css" && r.Method == "GET":
		b, _ := web.ReadFile("web/rows.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(b)
	case p == "/form.css" && r.Method == "GET":
		b, _ := web.ReadFile("web/form.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(b)
	case p == "/app.js" && r.Method == "GET":
		b, _ := web.ReadFile("web/app.js")
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(b)
	case p == "/healthz":
		writeJSON(w, 200, map[string]string{"status": "ok", "version": version})
	case p == "/v1/shares" && r.Method == "POST":
		s.createShare(w, r)
	case p == "/v1/shares" && r.Method == "GET":
		s.list(w, r)
	case p == "/v1/dashboard/shares" && r.Method == "GET":
		s.dashboardList(w, r)
	case strings.HasPrefix(p, "/v1/dashboard/shares/") && r.Method == "DELETE":
		s.dashboardRemove(w, r, strings.TrimPrefix(p, "/v1/dashboard/shares/"))
	case p == "/v1/dashboard/tokens" && r.Method == "GET":
		s.dashboardTokens(w, r)
	case p == "/v1/dashboard/tokens" && r.Method == "POST":
		s.dashboardCreateToken(w, r)
	case strings.HasPrefix(p, "/v1/dashboard/tokens/") && r.Method == "DELETE":
		s.dashboardRevokeToken(w, r, strings.TrimPrefix(p, "/v1/dashboard/tokens/"))
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
