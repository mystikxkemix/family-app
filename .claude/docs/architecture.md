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
