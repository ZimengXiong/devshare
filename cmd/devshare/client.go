package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func apiRequest(c clientConfig, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.URL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return http.DefaultClient.Do(req)
}

type clientConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func configPath() string {
	d, e := os.UserConfigDir()
	if e != nil {
		d = "."
	}
	return filepath.Join(d, "devshare", "config.json")
}

func client() clientConfig {
	c := clientConfig{URL: os.Getenv("DEVSHARE_URL"), Token: os.Getenv("DEVSHARE_TOKEN")}
	if b, e := os.ReadFile(configPath()); e == nil {
		var f clientConfig
		_ = json.Unmarshal(b, &f)
		if c.URL == "" {
			c.URL = f.URL
		}
		if c.Token == "" {
			c.Token = f.Token
		}
	}
	c.URL = normalizeURL(c.URL)
	if c.URL == "" || c.Token == "" {
		log.Fatal("authenticate with `devshare auth login --url ... --token ...` or set DEVSHARE_URL and DEVSHARE_TOKEN")
	}
	return c
}

func auth() {
	if len(os.Args) < 3 || os.Args[2] != "login" {
		log.Fatal("usage: devshare auth login --url URL --token TOKEN")
	}
	fs := flag.NewFlagSet("auth login", flag.ExitOnError)
	u := fs.String("url", "", "API URL")
	t := fs.String("token", "", "API token")
	_ = fs.Parse(os.Args[3:])
	if *u == "" || *t == "" {
		log.Fatal("--url and --token are required")
	}
	p := configPath()
	_ = os.MkdirAll(filepath.Dir(p), 0700)
	b, _ := json.MarshalIndent(clientConfig{URL: normalizeURL(*u), Token: *t}, "", "  ")
	if e := os.WriteFile(p, b, 0600); e != nil {
		log.Fatal(e)
	}
	fmt.Println("Authenticated with", *u)
}
