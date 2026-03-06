package main

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func publish() {
	fs, pub, keep, ttl := parseShareFlags("publish", os.Args[2:])
	_ = fs.Parse(os.Args[2:])
	if fs.NArg() != 1 {
		log.Fatal("usage: devshare publish <file-or-directory> [--public] [--keep|--ttl 2h]")
	}
	c := client()
	out := createRemote(c, "static", *pub, *keep, *ttl)
	id := out.ID
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
	req, _ := http.NewRequest("PUT", c.URL+"/v1/shares/"+id+"/content", pr)
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
	fmt.Println(out.URL)
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

func publishFormat(path string) string {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" {
			return "markdown"
		}
	}
	return "html"
}
