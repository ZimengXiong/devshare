package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"
)

func parseShareFlags(name string, args []string) (*flag.FlagSet, *bool, *bool, *string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	pub := fs.Bool("public", false, "allow anyone to view")
	keep := fs.Bool("keep", false, "never expire")
	ttl := fs.String("ttl", "", "expiration such as 2h")
	return fs, pub, keep, ttl
}

func createRemote(c clientConfig, kind string, public, keep bool, ttl string) shareResponse {
	visibility := "private"
	if public {
		visibility = "public"
	}
	body, _ := json.Marshal(struct {
		Kind, Visibility, TTL string
		Keep                  bool
	}{kind, visibility, ttl, keep})
	req, _ := http.NewRequest("POST", c.URL+"/v1/shares", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("server: %s: %s", resp.Status, string(b))
	}
	var out shareResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}
