package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDurationEnv(t *testing.T) {
	t.Setenv("DEVSHARE_TEST_TTL", "15m")
	if got := durationEnv("DEVSHARE_TEST_TTL", "1h"); got != 15*time.Minute {
		t.Fatalf("got %s", got)
	}
}

func TestNormalizeURL(t *testing.T) {
	if got := normalizeURL(" https://example.test/// "); got != "https://example.test" {
		t.Fatalf("got %q", got)
	}
}

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

func TestBearerToken(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "bearer  ds_test")
	if got := bearer(r); got != "ds_test" {
		t.Fatalf("got %q", got)
	}
}

func TestShareURLUsesConfiguredScheme(t *testing.T) {
	s := &Server{cfg: Config{PublicURL: "https://share.example.com"}}
	if got := s.shareURL("quiet-lake.localhost"); got != "https://quiet-lake.localhost" {
		t.Fatalf("got %q", got)
	}
}

func TestHostOnlyNormalizesTrailingDot(t *testing.T) {
	if got := hostOnly("SHARE.EXAMPLE."); got != "share.example" {
		t.Fatalf("got %q", got)
	}
}

func TestDashboardAssets(t *testing.T) {
	server := &Server{cfg: Config{DisableViewerAuth: true, PublicURL: "http://localhost:8080"}}
	for _, path := range []string{"/", "/style.css", "/rows.css", "/form.css", "/app.js"} {
		request := httptest.NewRequest("GET", path, nil)
		response := httptest.NewRecorder()
		server.control(response, request)
		if response.Code != 200 {
			t.Errorf("%s returned %d", path, response.Code)
		}
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

func TestPackMarkdownAsGitHubFlavoredHTML(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	source := "# Demo\n\n- [x] shipped\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := pack(path, &archive); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "site")
	if err := extractTarGz(bytes.NewReader(archive.Bytes()), dest); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dest, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"<title>Demo</title>", "type=\"checkbox\"", "<table>", "markdown-body"} {
		if !bytes.Contains(page, []byte(expected)) {
			t.Fatalf("rendered page does not contain %q", expected)
		}
	}
}
