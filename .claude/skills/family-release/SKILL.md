---
name: family-release
description: Ship the Family API to the ZimaOS server - push, watch CI and the image publish, follow the store bump and the Pages deploy, then check the server version and logs. Use when asked to deploy, release, publish, or when the server does not show the new version.
---

# Release to the server

Details and failure table: `.claude/docs/operations.md`. Server address and
accounts: CLAUDE.local.md. `export GH_TOKEN=$(gh auth token --user mystikxkemix)`.

1. After the merge/push to `master`, find the run:
   `gh run list -R mystikxkemix/family-app --commit $(git rev-parse HEAD) --limit 1`
   and watch it in the background:
   `gh run watch <id> -R mystikxkemix/family-app --exit-status`.
2. When "Publish image" passed, read the new tag:
   `gh api /users/mystikxkemix/packages/container/family-api/versions -q '.[0].metadata.container.tags'`.
3. The store bumps itself (hourly, or on any push to the store repo). Check:
   `gh api repos/mystikxkemix/zimaos-appstore/contents/Apps/family-api/docker-compose.yml -q .content | base64 -d | grep image:`
   then wait for the "pages build and deployment" run of the store repo.
4. Ask Mike to refresh the store and update Family API in ZimaOS (only he can).
5. Check: `curl -s http://<server>:8100/healthz` shows the new version. If the
   port is closed, ask Mike for `docker logs family-api` and match the
   `"msg":"stopped"` line with the failure table.
6. For a data check, read only (mongosh command in operations.md).
