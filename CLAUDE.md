# Family app

Go API (`api/`, with the website in `api/web/`) and SwiftUI iPhone app
(`ios/`). See README.md for setup, settings and security.

## Docs and skills (read the one for your task first)

| Topic | File |
|---|---|
| Code layout, data model, API conventions, iOS and web structure | `.claude/docs/architecture.md` |
| Release chain, server config, failures, git/gh from this Mac | `.claude/docs/operations.md` |
| iOS build, signing, localization, UI test pitfalls | `.claude/docs/ios.md` |
| Machine-specific values (server, devices, teams, accounts) | `CLAUDE.local.md` (gitignored, not in git) |

Skills (`.claude/skills/`): `family-design` (Idea → design → To validate),
`family-implement` (Approved → PR → In review), `family-feature` (checklist
across layers), `family-verify` (all checks), `family-release` (deploy to the
server), `family-install-phone`, `family-ui-tests`.

## Rules

- The repository is public: never commit personal data (team ids, bundle ids
  of real teams, IP addresses, emails, keys). Signing profiles live in
  gitignored `ios/Config/Signing/*.xcconfig`; secrets in the server's
  `settings.json`.
- Commit with the personal GitHub account (local git config: `kemix`).
- Every UI string exists in English and French (iOS `Localizable.xcstrings`,
  website `api/web/src/i18n`).
- API tests: `cd api && TEST_MONGODB_URI=mongodb://localhost:27099 go test -race ./...`
  with `docker run -d --rm -p 27099:27017 mongo:8`.

## Project management (GitHub Project)

Features go through the GitHub Project of the repository. Each feature is an
issue (template "Feature idea"), with labels for the area (`api`, `ios`, `web`,
`infra`). The Project's **Status** field:

| Status | Who | What happens |
|---|---|---|
| Idea | Mike | A new issue with the idea in a few words. |
| Design | Claude | Claude writes the design as an issue comment: goal, screens/endpoints, data, edge cases, tests, open questions. |
| To validate | Mike | Mike reads the design, answers the questions, asks for changes (back to Design) or approves. |
| Approved | Mike | Ready to build. Only approved items are implemented. |
| In progress | Claude | Branch `feat/<issue>-<slug>`, implementation, tests. |
| In review | Mike | Pull request open with "Closes #<issue>", CI green. Mike tests and merges. |
| Done | - | Set when the PR is merged (issue closed). |
| Won't do | Mike | Dropped. |

When asked to "work on the project": design the items in **Idea** (move them
to Design while writing, then To validate), and implement the items in
**Approved** (In progress, then In review with the PR). Never implement an item
that is not Approved, and never merge: Mike merges.
