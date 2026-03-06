package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorize(r, "admin"); !ok {
		http.Error(w, "admin scope required", http.StatusForbidden)
		return
	}
	var q tokenInput
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q) != nil || strings.TrimSpace(q.Label) == "" || !validScopes(q.Scopes) {
		http.Error(w, "label and valid scopes are required", http.StatusBadRequest)
		return
	}
	out, err := s.issueToken(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
