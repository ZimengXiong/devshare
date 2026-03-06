package main

import (
	"encoding/base64"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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
		log.Printf("OIDC code exchange failed: %v", e)
		http.Error(w, "login failed", 401)
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idtok, e := s.verifier.Verify(r.Context(), raw)
	if e != nil {
		log.Printf("OIDC identity verification failed: %v", e)
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
