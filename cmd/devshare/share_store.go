package main

import (
	"database/sql"
	"github.com/hashicorp/yamux"
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) getShare(id string) (Share, error) {
	var x Share
	err := s.db.QueryRow(`SELECT id,hostname,kind,visibility,owner_token_id,coalesce(tunnel_secret,''),expires_at,created_at FROM shares WHERE id=?`, id).Scan(&x.ID, &x.Hostname, &x.Kind, &x.Visibility, &x.OwnerTokenID, &x.TunnelSecret, &x.ExpiresAt, &x.CreatedAt)
	return x, err
}

func (s *Server) getShareTarget(target string) (Share, error) {
	var x Share
	err := s.db.QueryRow(`SELECT id,hostname,kind,visibility,owner_token_id,coalesce(tunnel_secret,''),expires_at,created_at FROM shares WHERE id=? OR hostname=?`, target, target).Scan(&x.ID, &x.Hostname, &x.Kind, &x.Visibility, &x.OwnerTokenID, &x.TunnelSecret, &x.ExpiresAt, &x.CreatedAt)
	return x, err
}

func (s *Server) owned(r *http.Request, share Share, scope string) bool {
	id, ok := s.authorize(r, scope)
	if !ok {
		return false
	}
	if id == share.OwnerTokenID {
		return true
	}
	_, admin := s.authorize(r, "admin")
	return admin
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
	shares := []shareResponse{}
	for rows.Next() {
		var share shareResponse
		var host string
		var expires sql.NullTime
		if err := rows.Scan(&share.ID, &host, &share.Kind, &share.Visibility, &expires, &share.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		share.URL = s.shareURL(host)
		if expires.Valid {
			share.ExpiresAt = &expires.Time
		}
		shares = append(shares, share)
	}
	writeJSON(w, http.StatusOK, shares)
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
