# devshare

Put a webpage on the internet from any machine. Devshare is private and
temporary by default. One statically linked binary is both the server and CLI.

```sh
devshare auth login --url https://share.example.com --token ds_...
devshare publish ./dist
devshare publish --public --ttl 2h ./dist
devshare serve --public 5173
devshare list
```

`publish` uploads a snapshot that survives after the CLI exits. `serve` opens
an outbound WebSocket tunnel to a local HTTP server. The same commands and API
keys work on laptops, CI workers, agents, and the devshare server itself.

## Five-minute self-hosting

1. Copy `.env.example` to `.env` and set the public API URL, site domain, and a
   random bootstrap token.
2. Run `docker compose up -d --build`.
3. Reverse proxy `share.example.com` and the unmatched `*.example.com`
   hostnames to port 8080.
4. Point wildcard DNS at the proxy.
5. For private shares, create an OIDC client with callback
   `https://share.example.com/auth/callback` and fill the three OIDC variables.

No PostgreSQL, object store, Redis, privileged container, Docker socket, or
host networking is required. State is a SQLite database plus extracted site
directories under `/var/lib/devshare`.

## Policy

API tokens are hashed at rest. Supplied invalid credentials always fail; there
is no anonymous publishing fallback. Uploaded archives reject traversal,
symlinks, devices, more than 5,000 files, and more than 256 MiB extracted data.
Private viewer authentication uses OIDC and exchanges the callback for a
host-only, HttpOnly session cookie on each generated origin.

The bootstrap token has all scopes for initial setup. Create narrower agent
tokens for routine use (token management UI/API is under active development).

## Scale

The default deployment deliberately optimizes for a one-command single-node
installation. SQLite WAL and local atomic directory swaps are appropriate for
personal and small-team deployments. A multi-node storage/database interface
is planned before claiming horizontal scale; the wire API and CLI do not need
to change when those backends are introduced.
