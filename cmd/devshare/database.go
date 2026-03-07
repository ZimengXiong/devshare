package main

import (
	"bytes"
	"os"
	"path/filepath"
	"time"
)

func (s *Server) migrate() error {
	_, e := s.db.Exec(`
CREATE TABLE IF NOT EXISTS tokens(id TEXT PRIMARY KEY, hash TEXT UNIQUE NOT NULL, label TEXT NOT NULL, scopes TEXT NOT NULL, created_at DATETIME NOT NULL, revoked_at DATETIME);
CREATE TABLE IF NOT EXISTS shares(id TEXT PRIMARY KEY, hostname TEXT UNIQUE NOT NULL, kind TEXT NOT NULL, format TEXT NOT NULL DEFAULT 'html', visibility TEXT NOT NULL, owner_token_id TEXT NOT NULL, tunnel_secret TEXT, expires_at DATETIME, created_at DATETIME NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY, email TEXT NOT NULL, expires_at DATETIME NOT NULL);
CREATE TABLE IF NOT EXISTS handoffs(code_hash TEXT PRIMARY KEY, email TEXT NOT NULL, hostname TEXT NOT NULL, return_path TEXT NOT NULL, expires_at DATETIME NOT NULL);
	`)
	if e == nil {
		_, _ = s.db.Exec(`ALTER TABLE shares ADD COLUMN format TEXT NOT NULL DEFAULT 'html'`)
	}
	return e
}

func (s *Server) backfillFormats() error {
	rows, err := s.db.Query(`SELECT id FROM shares WHERE kind='static' AND format='html'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var markdownIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		page, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "sites", id, "index.html"))
		if err == nil && bytes.Contains(page, []byte(`class="markdown-body"`)) {
			markdownIDs = append(markdownIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range markdownIDs {
		if _, err := s.db.Exec(`UPDATE shares SET format='markdown' WHERE id=?`, id); err != nil {
			return err
		}
	}
	return nil
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
