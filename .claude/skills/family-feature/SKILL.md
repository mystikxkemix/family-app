---
name: family-feature
description: Checklist to add or change a feature across the Family app layers (Go store, API handler, tests, push keys, iPhone models/views/strings, website types/pages/i18n). Use whenever a change touches the data model or more than one layer.
---

# Add a feature across the layers

Go through the list in order and skip what does not apply.

## API (`api/`)

1. Model in `internal/store/models.go` (bson + json tags; `householdId` with
   `json:"-"`; ObjectIDs; times from `store.Now()`).
2. Collection + index in `store.go` (`Open` + `ensureIndexes`), methods in
   `content.go` / `accounts.go` using `findAll`, `findOne`, `updateOne`,
   `deleteOne`, `insertOne`, and `scoped(householdID, id)` for every shared
   document.
3. Handler file in `internal/api/<area>.go`: request struct with pointer
   fields, `fields(creating bool)` validation with `cleanText` / `oneOf`,
   handlers returning `error`. Register routes in `server.go` with
   `s.authed`, `s.authedDuringPasswordChange` or `s.adminOnly`.
4. Push: `s.notifier.Notify` (or `NotifyBatched` for bursts) with English
   `Title`/`Body` AND `TitleLocKey`/`BodyLocKey` + args. Add every new
   `push.*` key to the iOS catalog (EN + FR, `extractionState: manual`).
5. Tests in `internal/api/api_test.go` (helpers: `newTestServer`, `signUp`,
   `client.call`, `strangerClient` for household isolation). Test validation
   errors and isolation, not only the happy path.

## iPhone (`ios/`)

6. Model in `Core/Models.swift` (Codable, same field names as the JSON).
7. Views in `Features/<Area>/`; API calls through `session.run { api in ... }`;
   reload with `.refreshable`, `.task` and `.onFamilyDataChanged`.
8. Accessibility identifiers on fields/buttons the UI test uses.
9. Strings: build, then make sure every extracted key has French:

   ```sh
   cd ios && python3 -c "
   import json,glob
   keys=set()
   for f in glob.glob('build/sim/**/*.stringsdata',recursive=True):
     for t,es in json.load(open(f)).get('tables',{}).items(): keys|={e['key'] for e in es}
   cat=json.load(open('FamilyApp/Resources/Localizable.xcstrings'))['strings']
   print([k for k in keys if 'fr' not in cat.get(k,{}).get('localizations',{})])"
   ```

   Add the missing ones to `Localizable.xcstrings`; remove keys no longer used
   (but keep `push.*`).

## Website (`api/web/`)

10. Types in `src/lib/types.ts`, calls in `src/lib/api.ts`, page in
    `src/features/<area>/`, strings in `src/i18n/en.ts` AND `fr.ts`.

## Docs

11. README (user-facing behavior, settings), `.claude/docs/*` when the
    architecture or a convention changes.
