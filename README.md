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
5. OIDC is required to access the `share.example.com` dashboard and private
   shares. create one confidential OIDC web client with:
   - authorization code enabled
   - callback `https://share.example.com/auth/callback`
   - scopes `openid profile email`
   - issuer, client ID, and client secret copied into `.env`

   public generated shares remain public and do not require OIDC.

authentik users can leave Signing Key empty or select a certificate; devshare
supports both. keycloak users should enable Client authentication and Standard
flow. set `DEVSHARE_OIDC_ISSUER`, `DEVSHARE_OIDC_CLIENT_ID`, and
`DEVSHARE_OIDC_CLIENT_SECRET`, then restart devshare.

devshare is made to be small, it's a SQLite database plus extracted site directories under `/var/lib/devshare`.
