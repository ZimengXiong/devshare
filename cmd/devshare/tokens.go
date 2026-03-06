package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) dashboardTokens(w http.ResponseWriter, r *http.Request) {
	if !s.viewerOK(r) {
		http.Error(w, "sign in required", 401)
		return
	}
	rows, err := s.db.Query(`SELECT id,label,scopes,created_at,revoked_at FROM tokens ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	tokens := []tokenResponse{}
	for rows.Next() {
		var token tokenResponse
		var scopes string
		var revoked sql.NullTime
		if err := rows.Scan(&token.ID, &token.Label, &scopes, &token.CreatedAt, &revoked); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		token.Scopes = strings.Split(scopes, ",")
		token.Revoked = revoked.Valid
		token.Bootstrap = token.ID == "tok_bootstrap"
		tokens = append(tokens, token)
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) issueToken(q tokenInput) (tokenResponse, error) {
	q.Label = strings.TrimSpace(q.Label)
	token, id := "ds_"+randomText(40), "tok_"+randomText(12)
	_, err := s.db.Exec(`INSERT INTO tokens(id,hash,label,scopes,created_at) VALUES(?,?,?,?,?)`, id, hash(token), q.Label, strings.Join(q.Scopes, ","), time.Now().UTC())
	return tokenResponse{ID: id, Token: token, Label: q.Label, Scopes: q.Scopes}, err
}

func (s *Server) dashboardCreateToken(w http.ResponseWriter, r *http.Request) {
	if !s.viewerOK(r) {
		http.Error(w, "sign in required", 401)
		return
	}
	var q tokenInput
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q) != nil || strings.TrimSpace(q.Label) == "" || !validScopes(q.Scopes) {
		http.Error(w, "label and valid scopes are required", 400)
		return
	}
	out, err := s.issueToken(q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 201, out)
}

func (s *Server) dashboardRevokeToken(w http.ResponseWriter, r *http.Request, id string) {
	if !s.viewerOK(r) {
		http.Error(w, "sign in required", 401)
		return
	}
	if id == "tok_bootstrap" {
		http.Error(w, "bootstrap token cannot be revoked here", 403)
		return
	}
	result, err := s.db.Exec(`UPDATE tokens SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, time.Now().UTC(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(204)
}
