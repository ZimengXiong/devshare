package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func publish() {
	fs, pub, keep, ttl := parseShareFlags("publish", os.Args[2:])
	update := fs.String("update", "", "replace an existing share by ID or URL")
	_ = fs.Parse(os.Args[2:])
	if fs.NArg() != 1 {
		log.Fatal("usage: devshare publish [--update <share-id-or-url>] [--public] [--keep|--ttl 2h] <file-or-directory>")
	}
	c := client()
	target := updateTarget(*update)
	if target == "" {
		target = createRemote(c, "static", *pub, *keep, *ttl).ID
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		_ = mw.WriteField("format", publishFormat(fs.Arg(0)))
		part, e := mw.CreateFormFile("content", "site.tar.gz")
		if e == nil {
			e = pack(fs.Arg(0), part)
		}
		_ = mw.Close()
		_ = pw.CloseWithError(e)
	}()
	req, _ := http.NewRequest("PUT", c.URL+"/v1/shares/"+url.PathEscape(target)+"/content", pr)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("upload: %s", b)
	}
	var out shareResponse
	if e := json.NewDecoder(resp.Body).Decode(&out); e != nil {
		log.Fatal(e)
	}
	fmt.Println(out.URL)
	if *update != "" {
		fmt.Println("updated")
		return
	}
	expiration := "temporary"
	if *keep {
		expiration = "no expiration"
	}
	visibility := "private"
	if *pub {
		visibility = "public"
	}
	fmt.Printf("%s · %s\n", visibility, expiration)
}

func updateTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	u, err := url.Parse(value)
	if err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	return value
}

func publishFormat(path string) string {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return "html"
	}
	if err != nil || !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" {
			return "markdown"
		}
	}
	return "html"
}
