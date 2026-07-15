package main

import (
	"database/sql"
	"encoding/json"
	"github.com/hashicorp/yamux"
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) dashboardVisibility(w http.ResponseWriter, r *http.Request, target string) {
	if !s.viewerOK(r) {
		http.Error(w, "sign in required", http.StatusUnauthorized)
		return
	}
	var input struct {
		Visibility string `json:"visibility"`
		Share      string `json:"share"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || (input.Visibility != "private" && input.Visibility != "public") {
		http.Error(w, "visibility must be private or public", http.StatusBadRequest)
		return
	}
	if input.Share != "" {
		target = input.Share
	}
	if target == "" {
		http.Error(w, "share is required", http.StatusBadRequest)
		return
	}
	result, err := s.db.Exec(`UPDATE shares SET visibility=? WHERE id=? OR hostname=?`, input.Visibility, target, target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dashboardList(w http.ResponseWriter, r *http.Request) {
	if !s.viewerOK(r) {
		http.Error(w, "sign in required", http.StatusUnauthorized)
		return
	}
	rows, err := s.db.Query(`SELECT id,hostname,kind,format,visibility,expires_at,created_at FROM shares ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	shares := []shareResponse{}
	for rows.Next() {
		var share shareResponse
		var host, format string
		var expires sql.NullTime
		if err := rows.Scan(&share.ID, &host, &share.Kind, &format, &share.Visibility, &expires, &share.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if expires.Valid {
			share.ExpiresAt = &expires.Time
		}
		share.URL = s.shareURL(host)
		share.Type = format
		share.Online = true
		if share.Kind == "tunnel" {
			share.Type = "proxy"
			_, share.Online = s.tunnels.Load(share.ID)
		}
		shares = append(shares, share)
	}
	writeJSON(w, http.StatusOK, shares)
}

func (s *Server) dashboardRemove(w http.ResponseWriter, r *http.Request, id string) {
	if !s.viewerOK(r) {
		http.Error(w, "sign in required", http.StatusUnauthorized)
		return
	}
	result, err := s.db.Exec(`DELETE FROM shares WHERE id=?`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		http.NotFound(w, r)
		return
	}
	_ = os.RemoveAll(filepath.Join(s.cfg.DataDir, "sites", id))
	if value, ok := s.tunnels.LoadAndDelete(id); ok {
		_ = value.(*yamux.Session).Close()
	}
	w.WriteHeader(http.StatusNoContent)
}
