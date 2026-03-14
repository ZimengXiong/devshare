package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"github.com/coreos/go-oidc/v3/oidc"
	"math/big"
	"net/http"
	"strings"
	"time"
)

type fallbackKeySet struct{ primary, fallback oidc.KeySet }

func (k fallbackKeySet) VerifySignature(ctx context.Context, jwt string) ([]byte, error) {
	payload, err := k.primary.VerifySignature(ctx, jwt)
	if err == nil {
		return payload, nil
	}
	return k.fallback.VerifySignature(ctx, jwt)
}

func hash(v string) string { x := sha256.Sum256([]byte(v)); return hex.EncodeToString(x[:]) }

func (s *Server) ensureBootstrap(tok string) error {
	_, e := s.db.Exec(`INSERT OR IGNORE INTO tokens(id,hash,label,scopes,created_at) VALUES(?,?,?,?,?)`, "tok_bootstrap", hash(tok), "bootstrap", "publish,public,keep,tunnel,list,delete,admin", time.Now().UTC())
	return e
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) >= 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func (s *Server) authorize(r *http.Request, scope string) (string, bool) {
	tok := bearer(r)
	if tok == "" {
		return "", false
	}
	var id, scopes string
	err := s.db.QueryRow(`SELECT id,scopes FROM tokens WHERE hash=? AND revoked_at IS NULL`, hash(tok)).Scan(&id, &scopes)
	if err != nil {
		return "", false
	}
	for _, v := range strings.Split(scopes, ",") {
		if v == scope || v == "admin" {
			return id, true
		}
	}
	return "", false
}

func randomText(n int) string {
	const a = "23456789abcdefghjkmnpqrstuvwxyz"
	b := make([]byte, n)
	max := big.NewInt(int64(len(a)))
	for i := range b {
		x, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err)
		}
		b[i] = a[x.Int64()]
	}
	return string(b)
}
