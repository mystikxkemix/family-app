---
name: family-design
description: Design the features in the "Idea" column of the Family app GitHub Project - write the design as an issue comment, then move the item to "To validate". Use when asked to design features, work on the project ideas, or "process the Idea column".
---

# Design the Idea items

Workflow and statuses: CLAUDE.md ("Project management"). The project number
and gh account are in CLAUDE.local.md. All `gh` calls use the personal account:
`export GH_TOKEN=$(gh auth token --user mystikxkemix)`.

## Steps

1. List the items: `gh project item-list <number> --owner mystikxkemix --format json`
   and keep those with Status = Idea.
2. For each item: move it to **Design** (see "Move an item" below), read the
   issue and its comments (`gh issue view <n> -R mystikxkemix/family-app --comments`).
3. Read the code it touches (`.claude/docs/architecture.md` first). Challenge
   the idea when it conflicts with the app (security, privacy, complexity):
   say so in the design.
4. Post the design as ONE issue comment (`gh issue comment <n> --body-file`),
   in simple English, with these parts:
   - **Goal**: one or two sentences.
   - **What the user sees**: iPhone and website, screen by screen.
   - **API**: new/changed endpoints with request/response JSON, error codes.
   - **Data**: new collections/fields, indexes, migration of old documents.
   - **Push**: new notifications and their `push.*` keys (EN + FR text).
   - **Edge cases and security**.
   - **Tests**: API tests, UI test steps.
   - **Size**: S / M / L, and what can be cut for a first version.
   - **Questions for Mike**: numbered, each with a recommended answer.
5. Add area labels (`api`, `ios`, `web`, `infra`), move the item to
   **To validate**, and report the list of designed items with their links.

Never start coding in this skill. Never move an item to Approved: Mike does.

## Move an item

```sh
gh project field-list <number> --owner mystikxkemix --format json   # Status field id + option ids
gh project item-edit --project-id <project-node-id> --id <item-id> \
  --field-id <status-field-id> --single-select-option-id <option-id>
```

The project node id: `gh project view <number> --owner mystikxkemix --format json -q .id`.
