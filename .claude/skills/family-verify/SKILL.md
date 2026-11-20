---
name: family-verify
description: Run every check of the Family app before a commit or a PR - Go format/vet/tests with a throwaway MongoDB, website typecheck/build, iOS simulator build and French strings check. Use before committing, before a PR, or when asked to "check everything".
---

# Verify everything

Run from the repo root. Report each step as pass/fail with the failing output.

```sh
# 1. API (throwaway MongoDB, never the production one)
docker run -d --rm --name family-test-mongo -p 27099:27017 mongo:8
cd api && test -z "$(gofmt -l .)" && go vet ./... \
  && TEST_MONGODB_URI=mongodb://localhost:27099 go test -race -count=1 ./...
docker stop family-test-mongo

# 2. Website
cd api/web && npm ci --no-fund --no-audit && npm run typecheck && npm run build

# 3. iPhone (simulator, no signing)
cd ios && xcodegen generate --quiet && xcodebuild build -project FamilyApp.xcodeproj \
  -scheme FamilyApp -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath build/sim CODE_SIGNING_ALLOWED=NO -quiet
```

4. French strings: the check in the `family-feature` skill must print `[]`.
5. Optional, for UI changes: the `family-ui-tests` skill.

Notes: `npm run build` rewrites `api/internal/webui/dist` (gitignored except
`.gitkeep`). Without a website build the Go tests still pass (the website
route answers 404 with a hint).
