package main

import "time"

var allowedScopes = map[string]bool{"publish": true, "public": true, "keep": true, "tunnel": true, "list": true, "delete": true, "admin": true}

type tokenInput struct {
	Label  string   `json:"label"`
	Scopes []string `json:"scopes"`
}

type tokenResponse struct {
	ID        string    `json:"id"`
	Token     string    `json:"token,omitempty"`
	Label     string    `json:"label"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	Revoked   bool      `json:"revoked"`
	Bootstrap bool      `json:"bootstrap"`
}

type shareResponse struct {
	ID           string     `json:"id"`
	URL          string     `json:"url"`
	Hostname     string     `json:"hostname,omitempty"`
	Kind         string     `json:"kind"`
	Type         string     `json:"type"`
	Visibility   string     `json:"visibility"`
	TunnelSecret string     `json:"tunnelSecret,omitempty"`
	Online       bool       `json:"online"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt,omitempty"`
}

func validScopes(scopes []string) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if !allowedScopes[scope] {
			return false
		}
	}
	return true
}
