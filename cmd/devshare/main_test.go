package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClientConfigRoundTrip(t *testing.T) {
	want := clientConfig{URL: "https://share.example.com", Token: "ds_secret"}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got clientConfig
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "../outside", Mode: 0600, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gz.Close()

	dest := t.TempDir()
	if err := extractTarGz(bytes.NewReader(archive.Bytes()), dest); err == nil {
		t.Fatal("expected traversal archive to be rejected")
	}
	if _, err := os.Stat(filepath.Join(dest, "..", "outside")); !os.IsNotExist(err) {
		t.Fatal("archive escaped destination")
	}
}
