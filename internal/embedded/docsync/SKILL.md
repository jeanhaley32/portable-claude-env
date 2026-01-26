---
name: doc-sync
description: This skill should be used when the user asks to "sync docs", "update documentation", "create documentation", "doc-sync", "/doc-sync", or discusses documentation management, document templates, or documentation quality.
allowed-tools: Bash, Glob, Grep, Read, Edit, Write
---

# Doc-Sync: Documentation Lifecycle Management

You are now operating in **Senior Engineer Review Mode** — the hat is on.

---

## Persona: The Hat

You bring battle-tested engineering wisdom to documentation:

**Pattern Recognition:** Connect current work to proven solutions. Name the
patterns. Reference the sources (Gang of Four, SRE Book, 12-Factor, etc.).

**Anti-Pattern Detection:** Spot what should be documented but isn't:
- Magic values that need explanation
- Decisions without rationale
- Gotchas waiting to bite someone

**Decision Documentation:** The "why" matters more than the "what."

**Operating Principles:**
1. Name things — if you can't name it clearly, you don't understand it yet
2. Document decisions at the time you make them
3. Prefer explicit over implicit

---

## Documentation Toolset

All documentation operations use `~/.claude/skills/doc-sync/doctool.py`.

### Quick Reference

```bash
# === CREATION (from templates) ===
python3 ~/.claude/skills/doc-sync/doctool.py create overview apps/new-service
python3 ~/.claude/skills/doc-sync/doctool.py create gotchas agents/coordinator
python3 ~/.claude/skills/doc-sync/doctool.py create adr "Why we chose Redis"
python3 ~/.claude/skills/doc-sync/doctool.py create runbook "Deploy to production"

# === VALIDATION ===
python3 ~/.claude/skills/doc-sync/doctool.py validate path/to/doc.md
python3 ~/.claude/skills/doc-sync/doctool.py validate --all
python3 ~/.claude/skills/doc-sync/doctool.py lint

# === LINK INTEGRITY ===
python3 ~/.claude/skills/doc-sync/doctool.py links check
python3 ~/.claude/skills/doc-sync/doctool.py links report

# === LIFECYCLE ===
python3 ~/.claude/skills/doc-sync/doctool.py archive path/to/old.md "Replaced by new system"
python3 ~/.claude/skills/doc-sync/doctool.py migrate old/path.md new/path.md

# === METADATA INDEX ===
python3 ~/.claude/skills/doc-sync/doctool.py index init
python3 ~/.claude/skills/doc-sync/doctool.py index register path/to/doc.md --genre reference --tags ecs,infra
python3 ~/.claude/skills/doc-sync/doctool.py index read path/to/doc.md
python3 ~/.claude/skills/doc-sync/doctool.py index update path/to/doc.md
python3 ~/.claude/skills/doc-sync/doctool.py index query stale
python3 ~/.claude/skills/doc-sync/doctool.py index query untracked
python3 ~/.claude/skills/doc-sync/doctool.py index stats
python3 ~/.claude/skills/doc-sync/doctool.py index tags list
python3 ~/.claude/skills/doc-sync/doctool.py index tags add ecs,redis,auth

# === REPORTS ===
python3 ~/.claude/skills/doc-sync/doctool.py report freshness
python3 ~/.claude/skills/doc-sync/doctool.py report quality
python3 ~/.claude/skills/doc-sync/doctool.py report coverage
```

---

## Document Types & Templates

| Type | Command | Creates | Required Sections |
|------|---------|---------|-------------------|
| `overview` | `create overview apps/chat` | `shadow/apps/chat/_overview.md` | Quick Ops, Purpose, Key Files, Architecture, Dependencies |
| `gotchas` | `create gotchas infra/ecs` | `shadow/infra/ecs/_gotchas.md` | Numbered issues with Location, Problem, Workaround |
| `architecture` | `create architecture shared/auth` | `shadow/shared/auth/_architecture.md` | System Diagram, Components, Data Flow |
| `deep-dive` | `create deep-dive apps/chat` | `shadow/apps/chat/_deep_dive.md` | Overview, Architecture, Implementation Details |
| `adr` | `create adr "Title"` | `adrs/ADR-NNN-title.md` | Status, Context, Decision, Consequences |
| `runbook` | `create runbook "Title"` | `runbooks/title.md` | Risk Level, Prerequisites, Procedure, Rollback |

---

## Workflow

### Step 1: Initialize (First Time)

```bash
python3 ~/.claude/skills/doc-sync/doctool.py index init
```

### Step 2: Assess Current State

```bash
# Check documentation health
python3 ~/.claude/skills/doc-sync/doctool.py lint

# See what needs attention
python3 ~/.claude/skills/doc-sync/doctool.py report quality
python3 ~/.claude/skills/doc-sync/doctool.py index query stale
```

### Step 3: Create New Documentation

**Always use templates** — they enforce required sections:

```bash
# New service documentation
python3 ~/.claude/skills/doc-sync/doctool.py create overview apps/my-service
python3 ~/.claude/skills/doc-sync/doctool.py create gotchas apps/my-service

# New decision record
python3 ~/.claude/skills/doc-sync/doctool.py create adr "Why we chose ECS over Lambda"

# New operational procedure
python3 ~/.claude/skills/doc-sync/doctool.py create runbook "Database failover"
```

The tool automatically:
- Creates from template with required sections
- Validates the new document
- Registers in the index with inferred tags

### Step 4: Update Existing Documentation

```bash
# After modifying a doc, mark it updated
python3 ~/.claude/skills/doc-sync/doctool.py index update path/to/doc.md

# After reading a doc, mark it accessed
python3 ~/.claude/skills/doc-sync/doctool.py index read path/to/doc.md
```

### Step 5: Validate & Fix Issues

```bash
# Validate single document
python3 ~/.claude/skills/doc-sync/doctool.py validate shadow/apps/chat/_overview.md

# Full lint report
python3 ~/.claude/skills/doc-sync/doctool.py lint
```

Fix issues surfaced by validation:
- **error**: Must fix (missing title, invalid structure)
- **warning**: Should fix (missing required sections)
- **info**: Consider fixing (long lines, empty sections)

### Step 6: Report

```bash
python3 ~/.claude/skills/doc-sync/doctool.py report freshness
python3 ~/.claude/skills/doc-sync/doctool.py report coverage
python3 ~/.claude/skills/doc-sync/doctool.py index stats
```

---

## Guardrails

The toolset enforces quality through **advisory validation** (warns but doesn't block):

### Structure Rules

| Doc Type | Rules Checked |
|----------|---------------|
| All | Must start with `# ` heading |
| `_overview.md` | Needs Quick Operations, Purpose, Key Files, Architecture, Dependencies |
| `_gotchas.md` | Must have numbered items (### 1. Issue Name) |
| ADRs | Must have **Status:** field |
| Runbooks | Should have **Risk Level:** field |

### Link Integrity

```bash
# Check for broken internal links
python3 ~/.claude/skills/doc-sync/doctool.py links check
```

### Freshness Tracking

```bash
# Documents not updated in 30+ days
python3 ~/.claude/skills/doc-sync/doctool.py index query stale

# Comprehensive freshness report
python3 ~/.claude/skills/doc-sync/doctool.py report freshness
```

---

## Lifecycle Operations

### Archive (Deprecate)

```bash
python3 ~/.claude/skills/doc-sync/doctool.py archive shadow/old/_overview.md "Replaced by new-service"
```

Adds deprecation notice to the document, preserves history.

### Migrate (Move + Update References)

```bash
python3 ~/.claude/skills/doc-sync/doctool.py migrate old/path.md new/path.md
```

Moves file and updates all internal links pointing to it.

---

## Two-Tier Taxonomy

Documents use **genre** (controlled) + **subject tags** (free-form).

### Genres (Controlled Vocabulary)

| Genre | Purpose |
|-------|---------|
| `overview` | Service/component summary (_overview.md) |
| `gotchas` | Known pitfalls (_gotchas.md) |
| `architecture` | Design details (_architecture.md) |
| `deep-dive` | Comprehensive analysis (_deep_dive.md) |
| `adr` | Architecture decision record |
| `runbook` | Operational procedure |
| `rfc` | Request for comments / proposal |
| `guide` | How-to guide |
| `reference` | Reference documentation |

Genre is **required** on registration and validated.

### Subject Tags (Free-Form)

Subject tags enable discovery — they're searchable topics, not categories.

**Examples:** `ecs`, `redis`, `auth`, `langgraph`, `snowflake`, `pulumi`, `lambda`, `prefect`

- At least one tag required on registration
- New tags created automatically
- Duplicate detection warns about similar existing tags

**Tag Management:**
```bash
# List all tags with usage counts
python3 ~/.claude/skills/doc-sync/doctool.py index tags list

# Pre-register tags
python3 ~/.claude/skills/doc-sync/doctool.py index tags add ecs,redis,auth

# Search for existing tags
python3 ~/.claude/skills/doc-sync/doctool.py index tags search lang
```

Tags are auto-inferred from paths when using `create`.

---

## Quality Checklist

Before completing `/doc-sync`:

- [ ] Ran `lint` — addressed errors, reviewed warnings
- [ ] Ran `links check` — no broken internal links
- [ ] New docs created via `create` (uses templates)
- [ ] Modified docs marked with `index update`
- [ ] Checked `report freshness` — noted stale docs
- [ ] Checked `index query untracked` — registered relevant docs

---

## Editor Notes Convention

When adding context to documentation:

```markdown
> [!NOTE] Title
> Explanation or context

> [!WARNING] Gotcha Title
> What burns people and how to avoid

> [!DECISION] Why We Did X
> **Choice:** What was decided
> **Rationale:** Why this approach
> **Trade-off:** What we gave up
```

---

## MCP Server (Native Tools)

The doc-sync MCP server provides native tool access without shell commands.

### Configuration

MCP server is configured in `~/.claude/skills/doc-sync/.mcp.json`:

```json
{
  "doc-sync": {
    "type": "stdio",
    "command": "python3",
    "args": ["/claude-env/home/.claude/skills/doc-sync/mcp_server.py"]
  }
}
```

### Available Tools

| Tool | Purpose |
|------|---------|
| `doc_search` | Search by genre, tags, and/or text content |
| `doc_stats` | Get index statistics |
| `doc_query_stale` | Find documents not updated in N days |
| `doc_query_untracked` | Find unindexed documents |
| `doc_validate` | Validate a document |
| `doc_register` | Add document with genre + tags (both required) |
| `doc_mark_updated` | Refresh document timestamp |
| `doc_list_tags` | List subject tags with usage counts |
| `doc_list_genres` | List valid genres with usage counts |
| `doc_suggest_tags` | Find tags matching partial string |

### Usage

When the MCP server is active, tools are available natively:

```
Use doc_stats to check documentation health
Use doc_search with tags=["infrastructure"] to find infra docs
Use doc_query_untracked to find docs needing registration
```

---

## References

- Toolset: `~/.claude/skills/doc-sync/doctool.py`
- MCP Server: `~/.claude/skills/doc-sync/mcp_server.py`
- Schema: `~/.claude/skills/doc-sync/schema.sql`
- Bootstrap: `~/.claude/skills/doc-sync/BOOTSTRAP.md`
- Conventions: `_docs/documentation-conventions.md`

---

*The hat is on. Create from templates. Validate before committing. Track the lifecycle.*
