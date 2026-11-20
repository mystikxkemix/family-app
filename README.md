# Family app

A private app for a household to share things: grocery and to-do lists, a
watchlist of movies and shows, and upcoming events. Each change sends a push
notification to the other members.

- `api/`: the API, in Go, with MongoDB. Published as a Docker image.
- `ios/`: the iPhone app, in SwiftUI. English and French.

## Features

| Area | What you can do |
|---|---|
| Lists | Shared lists (groceries, to-do, other). Add items fast, tap to check, clear checked items. |
| Watchlist | Movies and shows, with status (to watch, watching, watched) and who wants to watch. "Pick for us" chooses a title at random, titles that everyone wants first. |
| Events | Upcoming events grouped by day, with time, place and notes. |
| Household | Invite a person with a one-time code. Rename the household. |
| Push | A notification when someone else adds a list, an item, a title or an event. Ten groceries added in a row give one notification, not ten. |

## API

### Run it

```sh
cd api/deploy && docker compose up --build   # API on http://localhost:8100, with its own MongoDB
```

Or use the image `ghcr.io/mystikxkemix/family-api`, built by CI on each push to
`master` (tags `1.<commit count>` and `latest`).

### Settings (environment variables)

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `8080` | HTTP port. |
| `MONGODB_URI` | `mongodb://localhost:27017` | MongoDB connection string. |
| `MONGODB_DB` | `family-app` | Database name. |
| `SESSION_TTL_DAYS` | `180` | A login ends after this many days without use. |
| `CLIENT_IP_HEADER` | empty | Header with the real client IP when behind a proxy (`X-Real-IP`, `CF-Connecting-IP`, `X-Forwarded-For`). Only set it if the proxy always writes this header. |
| `APNS_CONFIG_FILE` | empty | APNs config file. Empty means no push. |

### First account

When the database has no user, the API writes a one-time **setup code** in its
logs:

```
{"level":"WARN","msg":"no account yet: create the first one in the app with this setup code","setupCode":"K7PM-2XQR-9TWB"}
```

In the app, choose **First setup** and type this code. After that, add people
with **Settings > Invite someone**: the other person chooses **Join** and types
the invite code.

### Security

The API is made to be reachable from the internet.

- **Put it behind HTTPS** (reverse proxy, Cloudflare Tunnel...). The app refuses
  plain `http://`, except for local network addresses.
- Passwords are hashed with Argon2id. Login tokens are 256-bit random values;
  the database only stores their SHA-256 hash.
- No open signup: the first account needs the setup code from the logs, the
  next ones need an invite code (one use, 7 days).
- Rate limits: per IP on every request, and stricter on login and signup
  (per IP and per username).
- Every query is scoped to the caller's household.
- Small request bodies only (64 KB), strict JSON, no CORS (the app is native),
  security headers on every answer.
- The image is distroless and runs as a non-root user.
- Keep MongoDB itself **off the internet**: only the API should be public.

### Push notifications

Apple sends pushes only with a key from the Apple team that signed the app.
The app can be signed by several teams, so the API takes one key per team.

1. In each Apple developer account: **Certificates, Identifiers & Profiles >
   Keys > +**, enable **Apple Push Notifications service (APNs)**, download
   `AuthKey_<KEYID>.p8`.
2. Put the `.p8` files and an `apns.json` next to each other, for example in
   `/DATA/AppData/family-api/apns/` on the server. Format:
   [`api/deploy/apns.example.json`](api/deploy/apns.example.json). `topics` are
   the app bundle ids of that team.
3. Mount the folder and set `APNS_CONFIG_FILE=/data/apns/apns.json`.

The app tells the API its bundle id and whether it is a development build
(APNs sandbox) or a TestFlight/App Store build (production).

### Tests

```sh
cd api
docker run -d --rm -p 27099:27017 mongo:8
TEST_MONGODB_URI=mongodb://localhost:27099 go test -race ./...
```

Without `TEST_MONGODB_URI`, the API tests are skipped and only the unit tests run.

## iPhone app

Requirements: Xcode, [XcodeGen](https://github.com/yonaskolb/XcodeGen)
(`brew install xcodegen`). The Xcode project is generated from
`ios/project.yml` and is not committed.

### Signing

Team ids and bundle ids are personal, so they are not in the repository. Each
phone can use a different Apple team: see
[`ios/Config/Signing/README.md`](ios/Config/Signing/README.md).

```sh
cp ios/Config/Signing/example.xcconfig ios/Config/Signing/me.xcconfig   # then fill it in
ios/Scripts/install.sh me    # builds and installs on the connected iPhone
```

### Languages

English and French, in `ios/FamilyApp/Resources/Localizable.xcstrings`. The
push texts are translated by the app too: the API sends a key (`push.*`), and
the phone shows the text in its own language.

## Roadmap

- Sonarr and Radarr: send a title of the watchlist to download, straight from
  the app (the watchlist already keeps TMDB/TVDB ids for this).
- Prowlarr: interactive search and manual grab.
- Add an event to the iPhone calendar.
