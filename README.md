# devshare

{you, agent} put {webpage, markdown, port} on internet from anywhere with one command

private and ephemeral by default

```sh
devshare auth login --url https://share.example.com --token ds_...

# upload a snapshot
devshare publish ./dist

# publish a markdown
devshare publish README.md

# proxy a port
devshare serve --public 5173
```

## self host

1. copy `.env.example` to `.env` and set the public API URL, site domain, and a
   random bootstrap token.
2. run `docker compose up -d --build`.
3. reverse proxy `share.example.com` and the unmatched `*.example.com`
   hostnames to port 8080.
4. point wildcard DNS at the proxy.
5. for private shares, create an OIDC client with callback
   `https://share.example.com/auth/callback` and fill the three OIDC variables.

devshare is made to be small, it's a SQLite database plus extracted site directories under `/var/lib/devshare`.
