package main

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

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
	writeJSON(w, http.StatusCreated, shareResponse{ID: shareID, Hostname: hostname, URL: s.shareURL(hostname), Visibility: q.Visibility, ExpiresAt: expires, TunnelSecret: secret})
}
