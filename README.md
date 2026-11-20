# Family app

A private app for a household to share things: grocery and to-do lists, a
watchlist of movies and shows, places to try, and upcoming events. Each change
sends a push notification to the other members.

- `api/`: the API, in Go, with MongoDB. Published as a Docker image.
- `ios/`: the iPhone app, in SwiftUI. English and French.

## Features

| Area | What you can do |
|---|---|
| Lists | Shared lists (groceries, to-do, other). Add items fast, tap to check, clear checked items. |
| Watchlist | Movies and shows, with status (to watch, watching, watched) and who wants to watch. "Pick for us" chooses a title at random, titles that everyone wants first. |
| Media tools | With Radarr/Sonarr: search a title to fill its poster, and send it to download. With Prowlarr: interactive search and manual grab. All optional. |
| Places | Restaurants, bars, activities to try. Who wants to go, "tried" with a 1-5 rating, open in Maps. |
| Events | Upcoming events grouped by day, with time, place and notes. "Add to my calendar" puts one in the iPhone calendar. |
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
| `SETTINGS_FILE` | `/data/settings.json` | The settings file with the secrets (see below). |

### Settings file (secrets)

Secrets and the addresses of other services are **not** environment
variables: they live in one local JSON file, `settings.json`, in the `/data`
folder of the container. Start from
[`api/deploy/settings.example.json`](api/deploy/settings.example.json).
Every part is optional: without the file, push and media tools are off.

| Part | Content |
|---|---|
| `apns` | Push keys, one per Apple team (see Push notifications). |
| `radarr` | `url`, `apiKey`, optional `qualityProfileId` and `rootFolder` (default: the first one in Radarr). |
| `sonarr` | Same as Radarr, for shows (Sonarr v4). |
| `prowlarr` | `url`, `apiKey`, for the interactive search. |

The container runs as user `65532`, and the file holds secrets:

```sh
sudo chown -R 65532 /DATA/AppData/family-api/data
sudo chmod 600 /DATA/AppData/family-api/data/settings.json
```

A wrong key name (for example `radar`) stops the API with an error, so a typo
never turns a feature off without telling you. The API reads the file at
start: restart it after a change.

The API keys of Radarr, Sonarr and Prowlarr stay on the server: the app never
sees them. Any member of the household can start a download.

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
2. Put the `.p8` files in the data folder, for example
   `/DATA/AppData/family-api/data/apns/`.
3. List them in the `apns` part of `settings.json`. A relative `keyFile` is
   read from the folder of `settings.json`. `topics` are the app bundle ids of
   that team.

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

### UI tests

```sh
ios/Scripts/ui-test.sh              # default simulator: iPhone 17
ios/Scripts/ui-test.sh "iPhone Air" # or another one
```

The script starts a throwaway MongoDB and API in Docker, writes the server
address and the setup code to `ios/build/ui-test/settings.json` (a local file,
read by the tests), and runs the tests in the simulator. The tests create the
first account, then use every screen: lists, watchlist, places, events and
settings, and check the tabs in French. Screenshots go to
`ios/build/ui-test/screenshots`. Needs Docker; they do not run in CI.

### Languages

English and French, in `ios/FamilyApp/Resources/Localizable.xcstrings`. The
push texts are translated by the app too: the API sends a key (`push.*`), and
the phone shows the text in its own language.

## ZimaOS

The app store entry is in
[mystikxkemix/zimaos-appstore](https://github.com/mystikxkemix/zimaos-appstore)
(`Apps/family-api`). It uses the MongoDB of the ZimaOS host and the data folder
`/DATA/AppData/family-api/data` (for `settings.json` and the APNs keys).
