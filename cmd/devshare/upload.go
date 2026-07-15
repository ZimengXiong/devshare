package main

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) upload(w http.ResponseWriter, r *http.Request, id string) {
	x, e := s.getShareTarget(id)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if !s.owned(r, x, "publish") {
		http.Error(w, "forbidden", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	f, _, e := r.FormFile("content")
	if e != nil {
		http.Error(w, "content archive required", 400)
		return
	}
	defer f.Close()
	format := r.FormValue("format")
	if format != "markdown" {
		format = "html"
	}
	tmp := filepath.Join(s.cfg.DataDir, "sites", "."+x.ID+"-"+randomText(6))
	if e = extractTarGz(f, tmp); e != nil {
		os.RemoveAll(tmp)
		http.Error(w, e.Error(), 400)
		return
	}
	dest := filepath.Join(s.cfg.DataDir, "sites", x.ID)
	old := dest + ".old"
	_ = os.RemoveAll(old)
	_ = os.Rename(dest, old)
	if e = os.Rename(tmp, dest); e != nil {
		_ = os.Rename(old, dest)
		http.Error(w, e.Error(), 500)
		return
	}
	_ = os.RemoveAll(old)
	_, _ = s.db.Exec(`UPDATE shares SET format=? WHERE id=?`, format, x.ID)
	writeJSON(w, 200, map[string]string{"id": x.ID, "url": s.shareURL(x.Hostname)})
}
