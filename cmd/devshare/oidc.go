package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type oidcMetadata struct {
	Issuer        string   `json:"issuer"`
	JWKS          string   `json:"jwks_uri"`
	ResponseTypes []string `json:"response_types_supported"`
	GrantTypes    []string `json:"grant_types_supported"`
	Algorithms    []string `json:"id_token_signing_alg_values_supported"`
}

func loadOIDCMetadata(issuer string) (oidcMetadata, error) {
	var metadata oidcMetadata
	resp, err := http.Get(issuer + "/.well-known/openid-configuration")
	if err != nil {
		return metadata, fmt.Errorf("OIDC discovery failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return metadata, fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return metadata, fmt.Errorf("OIDC discovery is invalid: %w", err)
	}
	if metadata.Issuer != issuer {
		return metadata, fmt.Errorf("OIDC issuer mismatch: configured %q but provider reports %q", issuer, metadata.Issuer)
	}
	if !hasValue(metadata.ResponseTypes, "code") || (len(metadata.GrantTypes) > 0 && !hasValue(metadata.GrantTypes, "authorization_code")) {
		return metadata, fmt.Errorf("OIDC provider is not ready: enable Authorization Code in the provider's grant types")
	}
	if metadata.JWKS == "" {
		return metadata, fmt.Errorf("OIDC discovery does not include jwks_uri")
	}
	return metadata, nil
}

func hasValue(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
