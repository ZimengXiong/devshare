package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type visibilityInput struct {
	Share      string `json:"share"`
	Visibility string `json:"visibility"`
}

func visibility() {
	if len(os.Args) != 4 || (os.Args[3] != "private" && os.Args[3] != "public") {
		log.Fatal("usage: devshare visibility <share-id-or-url> <private|public>")
	}
	c := client()
	body, _ := json.Marshal(visibilityInput{Share: updateTarget(os.Args[2]), Visibility: os.Args[3]})
	req, _ := http.NewRequest("POST", c.URL+"/v1/visibility", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		message, _ := io.ReadAll(resp.Body)
		log.Fatalf("visibility: %s", strings.TrimSpace(string(message)))
	}
	var out shareResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s · %s\n", out.URL, out.Visibility)
}

func (s *Server) setVisibility(w http.ResponseWriter, r *http.Request) {
	var input visibilityInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.Share == "" || (input.Visibility != "private" && input.Visibility != "public") {
		http.Error(w, "share and visibility (private or public) are required", http.StatusBadRequest)
		return
	}
	share, err := s.getShareTarget(input.Share)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !s.owned(r, share, "publish") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if input.Visibility == "public" {
		if _, ok := s.authorize(r, "public"); !ok {
			http.Error(w, "public scope required", http.StatusForbidden)
			return
		}
	}
	if _, err := s.db.Exec(`UPDATE shares SET visibility=? WHERE id=?`, input.Visibility, share.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, shareResponse{ID: share.ID, URL: s.shareURL(share.Hostname), Visibility: input.Visibility})
}
