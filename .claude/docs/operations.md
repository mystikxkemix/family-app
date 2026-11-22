# Operations: release, deploy, server

Machine-specific values (server IP, device ids, team ids, GitHub accounts) are
in `CLAUDE.local.md` (gitignored). Never write them in committed files.

## Release chain

1. Push to `master` → CI (`.github/workflows/ci.yml`): API tests (MongoDB
   service), website typecheck + build, iOS simulator build, then **Publish
   image**: `ghcr.io/mystikxkemix/family-api:2.<merges into master>` + `latest`.
2. The store repo `mystikxkemix/zimaos-appstore` (`Apps/family-api/`) runs
   "Publish store" hourly and on push: `scripts/bump_images.py` raises the image
   tag and `x-casaos.version`, then GitHub Pages redeploys the store.
3. On ZimaOS: refresh the store, update Family API. The Pages deploy must be
   finished first, or ZimaOS still sees the old version.

Check what runs: `curl -s http://<server>:8100/healthz` → `{"status":"ok","version":"1.N"}`.

## Server configuration

- Env (ZimaOS app settings): `MONGODB_URI`, `MONGODB_DB`, `ADMIN_USERNAME`,
  `ADMIN_PASSWORD` (8+ chars, only used while the DB has no account),
  `SESSION_TTL_DAYS`, `CLIENT_IP_HEADER`, `SETTINGS_FILE`.
- Secrets: `/DATA/AppData/family-api/data/settings.json` (+ `apns/*.p8`),
  mounted at `/data`. The container user is `65532`: the folder must be owned
  by it to save settings from the website (`sudo chown -R 65532 ...`).

## Reading the database (read-only checks)

MongoDB on the server has no auth on the LAN. For a quick read-only check:

```sh
docker run --rm mongo:8 mongosh "mongodb://<server>:27017/family-app" --quiet \
  --eval 'printjson(db.users.find({}, {username:1, isAdmin:1, mustChangePassword:1, _id:0}).toArray())'
```

Only read. Never write to the production database from a script.

## Common failures

| Symptom | Cause | Fix |
|---|---|---|
| Port 8100 closed, container restarts | Start error, see `docker logs family-api` | Read the `"msg":"stopped"` line |
| `ADMIN_PASSWORD: password must be at least 8 characters` | Short admin password, empty DB | Set 8+ chars in ZimaOS settings |
| `no account yet: set ADMIN_PASSWORD` | Empty DB, no password | Set it |
| `parse settings file ... unknown field` | Typo in settings.json | Fix the key name |
| Settings page: `not_writable` | `/data` read-only or owned by root | chown 65532, mount read-write |
| ZimaOS shows the old version | Store Pages deploy not finished | Wait for "pages build and deployment", refresh the store |

## Git and GitHub from this Mac

- Commit as the personal account (repo-local git config, user `kemix`).
- The SSH alias for the personal account can fail when the 1Password agent is
  locked. Push over HTTPS with the personal gh token instead (no config change):

  ```sh
  git -c credential.helper= -c 'credential.helper=!f() { echo username=x-access-token; echo "password=$(gh auth token --user mystikxkemix)"; }; f' \
    push https://github.com/mystikxkemix/family-app.git master
  ```

- `gh` commands for the personal account: `GH_TOKEN=$(gh auth token --user mystikxkemix) gh ...`.
  Do not switch the active gh account (the work account must stay active).
