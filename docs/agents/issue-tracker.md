# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues on `Kevin-wenyu/kbdiag`. Use the `gh` CLI for all operations.

## Conventions

- **Create an issue**: `gh issue create --title "..." --body "..."`. Use a heredoc for multi-line bodies.
- **Read an issue**: `gh issue view <number> --comments`, filtering comments by `jq` and also fetching labels.
- **List issues**: `gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'` with appropriate `--label` and `--state` filters.
- **Comment on an issue**: `gh issue comment <number> --body "..."`
- **Apply / remove labels**: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **Close**: `gh issue close <number> --comment "..."`

Infer the repo from `git remote -v` — `gh` does this automatically when run inside a clone.

## Pull requests as a triage surface

**PRs as a request surface: no.** kbdiag is pushed directly to `main` by a single maintainer (see CLAUDE.md's "Git / Deploy" section) — there are no external contributors and no PR workflow, so `/triage` should treat PRs as out of scope.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill says "fetch the relevant ticket"

Run `gh issue view <number> --comments`.

## Relationship to design docs

Design specs and plans are not kept as separate files. Requirements live in `docs/PRD.md`, the current plan in `.omc/plans/` (at most one), and anything larger than the plan's scope is published as a GitHub issue. The old shell-era specs are archived in git tag `shell-final`.
