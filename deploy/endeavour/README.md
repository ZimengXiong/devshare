# Endeavour deployment

This Compose project preserves the production `devshare` project, container,
external `proxy` network, and `/var/lib/devshare` application path. Runtime
configuration is local at `/srv/state/devshare/runtime.env`; SQLite and uploaded
site data live at `/srv/state/devshare/data`.

Validate and deploy from this directory:

```sh
docker compose config --quiet
docker compose up -d --build
```

Backups are owned by the private `homelab-infra` repository and run through the
`devshare-backup.timer` user unit. The previous monorepo Compose definition and
state remain the Phase 5 rollback checkpoint.
