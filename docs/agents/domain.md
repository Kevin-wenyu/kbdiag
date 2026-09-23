# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`docs/PRD.md`** — product principles (§3), command model (§4), output contract (§5), DS scenarios (§11).
- **`docs/queries.md`** — query catalog, probe_id registry, version column.
- **`CLAUDE.md`** — design rationale and conventions; its "活文档" table lists the only live docs.

Do not create `CONTEXT.md` or an ADR directory. New terms go into `docs/PRD.md`; design decisions ("why") go into `CLAUDE.md`. The live-doc table in `CLAUDE.md` is authoritative — adding any other doc requires changing that table first.

## Use the project's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `docs/PRD.md` — e.g. finding, probe_id, verdict, `not_applicable`, `redacted`, the 看/查/断 depth levels. Don't drift to synonyms.

If the concept you need isn't defined yet, either you're inventing language the project doesn't use (reconsider) or there's a real gap (propose adding it to `docs/PRD.md`).

## Flag conflicts

If your output contradicts a documented decision in `CLAUDE.md` or the contract in `docs/PRD.md`, surface it explicitly rather than silently overriding.
