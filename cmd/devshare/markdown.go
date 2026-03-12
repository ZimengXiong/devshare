package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

func packMarkdown(tw *tar.Writer, path string) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	markdown := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	var rendered bytes.Buffer
	if err = markdown.Convert(source, &rendered); err != nil {
		return err
	}
	css, err := web.ReadFile("web/markdown.css")
	if err != nil {
		return err
	}
	title := markdownTitle(path, source)
	page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>%s</style>
</head>
<body>
  <main class="markdown-body">
%s
  </main>
</body>
</html>
`, template.HTMLEscapeString(title), css, rendered.String())
	h := &tar.Header{Name: "index.html", Mode: 0644, Size: int64(len(page)), ModTime: time.Now()}
	if err = tw.WriteHeader(h); err != nil {
		return err
	}
	_, err = io.WriteString(tw, page)
	return err
}

func markdownTitle(path string, source []byte) string {
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if lines := strings.Split(string(source), "\n"); len(lines) > 0 && strings.HasPrefix(lines[0], "# ") {
		title = strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
	}
	return title
}
