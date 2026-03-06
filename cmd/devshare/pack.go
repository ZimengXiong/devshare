package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func pack(path string, w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	info, e := os.Stat(path)
	if e != nil {
		return e
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(path), ".md") || strings.EqualFold(filepath.Ext(path), ".markdown") {
			e = packMarkdown(tw, path)
		} else if strings.EqualFold(filepath.Ext(path), ".html") || strings.EqualFold(filepath.Ext(path), ".htm") {
			e = packFile(tw, path, "index.html", info)
		} else {
			e = errors.New("a single file must be HTML or Markdown")
		}
		if ce := tw.Close(); e == nil {
			e = ce
		}
		if ce := gz.Close(); e == nil {
			e = ce
		}
		return e
	}
	root := path
	e = filepath.Walk(root, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if i.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported: %s", p)
		}
		h, e := tar.FileInfoHeader(i, "")
		if e != nil {
			return e
		}
		h.Name = filepath.ToSlash(rel)
		if e = tw.WriteHeader(h); e != nil {
			return e
		}
		if i.Mode().IsRegular() {
			f, e := os.Open(p)
			if e != nil {
				return e
			}
			_, e = io.Copy(tw, f)
			_ = f.Close()
			return e
		}
		return nil
	})
	if ce := tw.Close(); e == nil {
		e = ce
	}
	if ce := gz.Close(); e == nil {
		e = ce
	}
	return e
}

func packFile(tw *tar.Writer, path, name string, info os.FileInfo) error {
	h, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	h.Name = name
	if err = tw.WriteHeader(h); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}
