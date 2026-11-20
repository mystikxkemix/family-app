---
name: family-implement
description: Implement the "Approved" items of the Family app GitHub Project - branch, code, tests, pull request, then move the item to "In review". Use when asked to build approved features or "work on the Approved column".
---

# Implement the Approved items

Read CLAUDE.md ("Project management") and `.claude/docs/architecture.md`.
`gh` uses the personal account: `export GH_TOKEN=$(gh auth token --user mystikxkemix)`.

## Steps (one item at a time)

1. Only items with Status = **Approved**. Read the issue, the design comment
   and Mike's answers. The design + answers are the spec: when something is
   unclear, ask (AskUserQuestion) instead of guessing.
2. Move the item to **In progress** (see family-design, "Move an item").
3. `git switch master && git pull` then `git switch -c feat/<issue>-<slug>`.
4. Build it with the `family-feature` checklist (all layers, EN + FR strings,
   tests).
5. Run `family-verify`. Everything must pass.
6. Scan for personal data before committing:
   `grep -rniE "<values from CLAUDE.local.md>" --exclude-dir=build --exclude-dir=.git --exclude-dir=node_modules .`
7. Commit (conventional commits, `kemix` identity), push the branch over
   HTTPS (see `.claude/docs/operations.md`), open the PR:
   `gh pr create -R mystikxkemix/family-app --base master --title "feat(...): ..." --body "... Closes #<issue>"`.
   The PR body: what changed, how to test it on the phone/website, screenshots
   paths if any.
8. Watch CI (`gh run watch <id> --exit-status` in the background) and fix
   failures.
9. Move the item to **In review** and report the PR link. Never merge.
