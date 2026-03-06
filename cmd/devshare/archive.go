package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractTarGz(src io.Reader, dest string) error {
	gz, e := gzip.NewReader(src)
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count := 0
	var total int64
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		count++
		if count > 5000 {
			return errors.New("too many files")
		}
		name := filepath.Clean(h.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return errors.New("unsafe archive path")
		}
		target := filepath.Join(dest, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if e = os.MkdirAll(target, 0750); e != nil {
				return e
			}
		case tar.TypeReg:
			total += h.Size
			if total > 256<<20 {
				return errors.New("archive expands beyond 256 MiB")
			}
			if e = os.MkdirAll(filepath.Dir(target), 0750); e != nil {
				return e
			}
			f, e := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
			if e != nil {
				return e
			}
			_, e = io.CopyN(f, tr, h.Size)
			ce := f.Close()
			if e != nil {
				return e
			}
			if ce != nil {
				return ce
			}
		default:
			return errors.New("archive contains unsupported entry")
		}
	}
	if _, e = os.Stat(filepath.Join(dest, "index.html")); e != nil {
		return errors.New("archive must contain index.html at its root")
	}
	return nil
}
