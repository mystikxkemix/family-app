# Architecture

## Layout

```
api/                      Go API (module "familyapp"), Docker image ghcr.io/mystikxkemix/family-api
  cmd/family-api/main.go  start: env config, settings file, MongoDB, server
  internal/config         env (plain settings) + settings.json (secrets), atomic save
  internal/store          MongoDB: models.go (all types), accounts.go, content.go
  internal/auth           Argon2id passwords, random tokens/codes, rate limiter
  internal/push           APNs client (ES256 JWT, stdlib) + Notifier (async, batching)
  internal/arr            Radarr / Sonarr / Prowlarr clients
  internal/tmdb           TMDB client: search, details, safe automatic matching
  internal/api            HTTP: server.go (routes, middlewares), one file per area
  internal/webui          embeds the website build (dist/, only .gitkeep in git)
  web/                    website: React 19 + TypeScript + Vite, builds into internal/webui/dist
ios/                      SwiftUI app, XcodeGen (project.yml), iOS 18+, Swift 6
.github/workflows/ci.yml  API tests, website build, iOS simulator build, image publish
```

## Data model (internal/store/models.go)

Everything shared belongs to a **household** and every query filters on
`householdId` (`scoped()` helper). Collections: `households`, `users`,
`sessions`, `invites`, `devices`, `lists`, `list_items`, `media`, `places`,
`events`. IDs are Mongo ObjectIDs (hex strings in JSON). Indexes are created at
start in `store.ensureIndexes`; TTL indexes expire sessions and invites.

User flags: `mustChangePassword` (blocks everything except profile, password,
logout, push devices) and `isAdmin` (admin section of the website).

## HTTP API (internal/api)

- Routes: `server.go` `Handler()`. Base path `/api/v1`. Errors are
  `{"error": "...", "code": "..."}` (`apiError` in httputil.go); clients test
  the `code`.
- Wrappers: `s.handle` (public), `s.authed` (logged in),
  `s.authedDuringPasswordChange`, `s.adminOnly`.
- Request bodies: pointer fields = PATCH semantics; `fields()` methods validate
  and return the `$set` document. `decodeJSON` refuses unknown fields.
- Auth: iPhone = `Authorization: Bearer <token>`. Website = HttpOnly cookie
  `family_session` **plus** header `X-Requested-With: family-web` (CSRF guard).
  Only SHA-256 hashes of tokens are stored.
- First account: `ADMIN_USERNAME` / `ADMIN_PASSWORD` when the DB is empty;
  admin, must change password. Next accounts: invite codes only.
- Settings (`settings.json`): APNs keys + *arr URLs/keys. Loaded at start,
  editable live from the admin page (`adminPutSettings` → `applySettingsLocked`
  swaps the arr clients and the APNs sender).
- Push texts: the API sends `loc-key` + `loc-args`; the iPhone translates them
  from `Localizable.xcstrings` (keys `push.*`). A new push needs its key in the
  iOS catalog in EN and FR.
- Website: `website.go` serves `internal/webui` files, SPA fallback to
  index.html, its own CSP (`websiteCSP`); `/api/*` keeps the strict API CSP.

## Movie details (TMDB)

- `internal/tmdb`: v3 API; the key can be an API key (`api_key` param) or a
  v4 read access token (Bearer). Language: `fr-FR` or `en-US` from `lang` /
  `Accept-Language`. TMDB also gives the IMDb and TVDB ids: no TVDB or IMDb API.
- `GET /watchlist/{id}/details` reads the `media_details` cache (one entry per
  kind + TMDB id + language, TTL 30 days) before TMDB. `404 not_matched` when
  the title has no TMDB id.
- Automatic matching (`details.go`, `startMatcher`): a background goroutine
  runs at start, after a title is added without a TMDB id, and after a new
  TMDB key is saved. It keeps a match only if the normalized title (French,
  English or original) is equal and the year is within ±1, and sets
  `matchCheckedAt` either way so a title is tried once. `POST /watchlist/{id}/match`
  is the manual fix ("Change match").
- Local checks without a real key: `TMDB_BASE_URL` (dev only) points the API
  to a fake TMDB server.

## Jellyfin sync

- `internal/jellyfin`: read-only client (`Authorization: MediaBrowser Token="<key>"`).
  `Library(userID)` reads `/Items` (Movie, Series; pages of 500) and the played
  episodes: a show has no `LastPlayedDate`, its date is the one of its last
  played episode. Items without a TMDB and a TVDB id are skipped (YouTube
  downloads). A partly played show has `PlayedPercentage > 0`.
- One Jellyfin account per household: `households.jellyfinUserId`, set with
  `PUT /admin/jellyfin/household` (the admin's own household).
- `internal/api/jellyfin.go`: a goroutine syncs at start, every 15 minutes,
  and when the settings or the household link change; `POST /admin/jellyfin/sync`
  runs one now (`409 sync_running` if one runs). The last result is kept in
  memory (`GET /admin/jellyfin`).
- Match: TMDB id, then TVDB id, then normalized title + year ±1 (never when
  both have a different TMDB id). Matched titles get `media.jellyfin`
  (`itemId`, `available`, `playedAt`, `state`) and the missing TMDB/TVDB ids.
- Status: only forward (`statusRank`). `media.statusSetBy` = `user` (app,
  website) or `jellyfin`. A user's status is kept while `jellyfin.state` stays
  the same; a new played state moves it again.
- Import: fully played items not in the watchlist are added as Watched with
  `source: "jellyfin"`, no interested people, empty `addedBy`. Their poster
  comes from TMDB after the sync (`fillImportedPosters`, sets `matchCheckedAt`).
  Deleting a title linked to Jellyfin adds its item to
  `households.jellyfinIgnoredItems`: it is never imported again.
- Tests turn the goroutine off (`jellyfinAutoSync = false`) and call the sync
  route; the UI tests use a fake Jellyfin in nginx (`ios/Scripts/fake-jellyfin.conf`).
- A Go binary built on this Mac may get "no route to host" to LAN addresses
  (macOS Local Network privacy), while curl works: check the real Jellyfin
  with curl, not with `go run`.

## iPhone app (ios/)

- `Core/`: `APIClient` (value type, `JSONBody`/`JSONValue` for PATCH bodies),
  `AppSession` (@Observable: server URL, token in Keychain, `me`, error alert,
  `run { api in ... }` helper that handles 401 and password_change_required),
  `PushManager`, `Models.swift` (mirrors the Go models), `PasswordField.swift`.
- `Features/<Area>/`: one folder per tab (Lists, Watchlist, Places, Events,
  Settings) + Auth (login, join, forced password change).
- Screens reload on `.familyDataChanged` (push received, app reopened).
- Strings: SwiftUI literals are keys of `Resources/Localizable.xcstrings`
  (EN source + FR). Non-literal text: `LocalizedStringKey(...)` or
  `String(localized:)`.
- Signing: see [ios.md](ios.md).

## Website (api/web)

- React 19 + TypeScript + Vite, no UI library, plain CSS (`src/styles.css`,
  light/dark). Builds into `api/internal/webui/dist` (a Vite plugin keeps
  `.gitkeep`). Dev: `npm run dev` proxies `/api` to localhost:8080. This Mac's
  npm skips dev packages: use `npm ci --include=dev`.
- `src/lib/api.ts`: one typed `request<T>()`; always `credentials:
  "same-origin"` + header `X-Requested-With: family-web`; errors carry the API
  `code`. `src/lib/types.ts` mirrors the Go models. `src/lib/session.tsx`:
  login state, `me`, the forced password change. `src/lib/hooks.ts`: tiny
  router (`navigate`, `usePath`) and polling (on focus + every 30 s).
  `src/lib/ics.ts`: "Add to my calendar" files.
- Pages: `src/features/{auth,lists,watchlist,places,events,settings,admin}/`.
  Routes: `/lists`, `/lists/<id>`, `/watchlist`, `/places`, `/events`,
  `/settings`, `/admin` (admins only). The API answers index.html for them.
- Strings: `src/i18n/en.ts` and `fr.ts` (same keys), `t('key', {name})`;
  language from the browser, switch in Settings (localStorage).
- To look at it: run the binary with a throwaway MongoDB and drive Chrome with
  `puppeteer-core` (installed outside the repo), log in through the form, add
  data with `fetch` in the page, screenshot each route.
