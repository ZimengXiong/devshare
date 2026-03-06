package main

import (
	"log"
	"os"
	"strings"
	"time"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func loadConfig() Config {
	return Config{
		Listen:            env("DEVSHARE_LISTEN", ":8080"),
		PublicURL:         strings.TrimRight(env("DEVSHARE_PUBLIC_URL", "http://localhost:8080"), "/"),
		SiteDomain:        env("DEVSHARE_SITE_DOMAIN", "localhost"),
		DataDir:           env("DEVSHARE_DATA_DIR", "./data"),
		BootstrapToken:    os.Getenv("DEVSHARE_BOOTSTRAP_TOKEN"),
		DefaultTTL:        durationEnv("DEVSHARE_DEFAULT_TTL", "24h"),
		MaxTTL:            durationEnv("DEVSHARE_MAX_TTL", "168h"),
		OIDCIssuer:        os.Getenv("DEVSHARE_OIDC_ISSUER"),
		OIDCClientID:      os.Getenv("DEVSHARE_OIDC_CLIENT_ID"),
		OIDCClientSecret:  os.Getenv("DEVSHARE_OIDC_CLIENT_SECRET"),
		DisableViewerAuth: os.Getenv("DEVSHARE_DISABLE_VIEWER_AUTH") == "true",
	}
}

func durationEnv(k, d string) time.Duration {
	v, e := time.ParseDuration(env(k, d))
	if e != nil {
		log.Fatalf("%s: %v", k, e)
	}
	if v <= 0 {
		log.Fatalf("%s must be positive", k)
	}
	return v
}
