#!/usr/bin/env python3
"""
Documentation Toolset - Full lifecycle management for _docs/

Usage:
    doctool create <type> <name> [--tags t1,t2]   # Create from template
    doctool validate <path>                        # Validate single doc
    doctool validate --all                         # Validate all docs
    doctool lint                                   # Find all issues
    doctool links check                            # Check internal links
    doctool links report                           # Full link report
    doctool archive <path> [reason]                # Mark as deprecated
    doctool migrate <old> <new>                    # Move + update refs
    doctool index init                             # Initialize database
    doctool index register <path> --genre <g> --tags t1,t2
    doctool index read <path>                      # Mark as accessed
    doctool index update <path>                    # Mark as modified
    doctool index query stale|untracked|all|tag   # Query index
    doctool index stats                            # Show statistics
    doctool index tags list|add|search             # Manage subject tags
    doctool report freshness|quality|coverage      # Generate reports

    # Memory system (FTS5-based retrieval for AI collaboration)
    doctool memory ingest <path>                   # Add new document to memory
    doctool memory refresh <path>                  # Update existing document chunks
    doctool memory refresh --all                   # Rebuild all doc chunks from files
    doctool memory add <content> --tags t1,t2      # Add a note/memory directly
    doctool memory add --tags t1,t2 < file.txt     # Add from stdin
    doctool memory search <query> [--tags t1,t2]   # Search memory by text/tags
    doctool memory recent [--days N] [--limit N]   # List recent additions
    doctool memory stats                           # Show memory statistics

Genres: overview, gotchas, architecture, deep-dive, adr, runbook, rfc, guide, reference
Tags: Free-form subject tags (at least one required on register)
"""

import sqlite3
import sys
import os
import re
import hashlib
from datetime import datetime
from pathlib import Path
from typing import Optional
from dataclasses import dataclass, field

# =============================================================================
# CONFIGURATION
# =============================================================================

DB_PATH = Path("/workspace/_docs/.doc-index.db")
SCHEMA_PATH = Path(os.path.expanduser("~/.claude/skills/doc-sync/schema.sql"))
DOCS_ROOT = Path("/workspace/_docs")

# Controlled vocabulary for document genres
VALID_GENRES = [
    "overview",      # _overview.md - service/component summary
    "gotchas",       # _gotchas.md - known pitfalls
    "architecture",  # _architecture.md - design details
    "deep-dive",     # _deep_dive.md - comprehensive analysis
    "adr",           # ADR-NNN - architecture decision record
    "runbook",       # operational procedure
    "rfc",           # request for comments / proposal
    "guide",         # how-to guide
    "reference",     # reference documentation
]

# Required sections by document type
REQUIRED_SECTIONS = {
    "overview": [
        "Quick Operations",
        "Purpose",
        "Key Files",
        "Architecture",
        "Dependencies",
    ],
    "gotchas": [],  # Just needs numbered items
    "architecture": [
        "System Diagram",
        "Components",
    ],
    "deep-dive": [
        "Overview",
        "Architecture",
    ],
    "adr": [
        "Status",
        "Context",
        "Decision",
        "Consequences",
    ],
    "runbook": [
        "Risk Level",
        "Prerequisites",
        "Procedure",
        "Rollback",
        "Verification",
    ],
}

# =============================================================================
# DATA CLASSES
# =============================================================================

@dataclass
class ValidationIssue:
    """A single validation issue."""
    path: str
    severity: str  # error, warning, info
    rule: str
    message: str
    line: Optional[int] = None

@dataclass
class ValidationResult:
    """Result of validating a document."""
    path: str
    doc_type: str
    issues: list = field(default_factory=list)

    @property
    def is_valid(self) -> bool:
        return not any(i.severity == "error" for i in self.issues)

    @property
    def error_count(self) -> int:
        return sum(1 for i in self.issues if i.severity == "error")

    @property
    def warning_count(self) -> int:
        return sum(1 for i in self.issues if i.severity == "warning")

# =============================================================================
# TEMPLATES
# =============================================================================

TEMPLATES = {
    "overview": '''# {title}

## Quick Operations

| I need to... | Command / Action |
|--------------|------------------|
| Run locally | `just docker-up {path}` |
| Deploy | `just deploy {path} dev` |
| View logs | `just logs {path}` |

---

## Purpose

{description}

---

## Key Files

| File | Purpose |
|------|---------|
| `src/main.py` | Entry point |

---

## Architecture

```
┌─────────────┐
│   {name}    │
└─────────────┘
```

---

## Dependencies

### Depends On
- (list dependencies)

### Depended On By
- (list dependents)

---

## How to Make Changes

1. (step 1)
2. (step 2)

---

## Cross-References

- [Related Doc](../related/_overview.md)
''',

    "gotchas": '''# {title} Gotchas

> Known pitfalls and edge cases for {name}.

---

## Critical Issues

### 1. (Issue Name)

**Location:** `path/to/file.py:line`

**Problem:** (What goes wrong)

**Symptoms:**
- (Observable symptom 1)
- (Observable symptom 2)

**Workaround:** (How to avoid or fix)

---

## Medium Severity

(Add issues as discovered)

---

## Low Severity

(Add issues as discovered)
''',

    "architecture": '''# {title} Architecture

## System Diagram

```
┌─────────────────────────────────────────┐
│              {name}                      │
├─────────────────────────────────────────┤
│                                          │
│   (Add architecture diagram here)        │
│                                          │
└─────────────────────────────────────────┘
```

---

## Components

### Component 1

**Purpose:** (What it does)

**Key Files:**
- `path/to/file.py`

---

## Data Flow

1. (Step 1)
2. (Step 2)

---

## Design Decisions

> [!DECISION] Why This Architecture
> **Choice:** (What was decided)
> **Rationale:** (Why this approach)
> **Trade-off:** (What we gave up)
''',

    "deep-dive": '''# {title} Deep Dive

> Comprehensive analysis of {name}.

---

## Overview

{description}

---

## Architecture

(Detailed architecture explanation)

---

## Implementation Details

### Key Algorithms

(Explain core algorithms)

### State Management

(Explain state handling)

---

## Edge Cases

(Document edge cases and how they're handled)

---

## Performance Considerations

(Note any performance-relevant details)

---

## Related Documentation

- [Overview](../_overview.md)
- [Gotchas](../_gotchas.md)
''',

    "adr": '''# ADR-{number}: {title}

**Status:** Proposed
**Date:** {date}
**Author:** (Author name)

---

## Context

(What is the issue that we're seeing that motivates this decision?)

---

## Decision

(What is the change that we're proposing and/or doing?)

---

## Consequences

### Positive
- (Benefit 1)

### Negative
- (Trade-off 1)

### Neutral
- (Observation 1)

---

## Alternatives Considered

| Option | Pros | Cons | Why Not |
|--------|------|------|---------|
| (Alt 1) | (Pros) | (Cons) | (Reason) |
''',

    "runbook": '''# {title}

> **Risk Level:** Medium
> **Prerequisites:** (What must be true before starting)
> **Estimated Duration:** (Time range)

---

## When to Use

- (Scenario that triggers this procedure)

---

## Pre-flight Checks

- [ ] Verify (requirement 1)
- [ ] Confirm (requirement 2)
- [ ] Notify team (if applicable)

---

## Procedure

### Step 1: (Action)

**Purpose:** (What this accomplishes)

```bash
command here
```

**Expected output:**
```
what you should see
```

**If unexpected:** (What to do if output differs)

### Step 2: (Action)

(Continue with steps...)

---

## Rollback

If something goes wrong:

1. (First rollback step)
2. (Second rollback step)

---

## Verification

- [ ] Confirm (success criteria 1)
- [ ] Verify (success criteria 2)
''',
}

# =============================================================================
# DATABASE (Index Operations)
# =============================================================================

class DocIndex:
    """Handles document metadata in SQLite."""

    def __init__(self):
        self.db_path = DB_PATH
        self.schema_path = SCHEMA_PATH

    def get_connection(self):
        conn = sqlite3.connect(self.db_path)
        conn.execute("PRAGMA foreign_keys = ON")
        conn.row_factory = sqlite3.Row
        return conn

    def init_db(self):
        with open(self.schema_path) as f:
            schema = f.read()
        conn = self.get_connection()
        conn.executescript(schema)
        conn.commit()
        conn.close()
        print(f"Database initialized at {self.db_path}")

    def register(self, path: str, genre: str, tags: list, title: str = None):
        """Register a document. Genre (controlled) and tags (free-form) required."""
        # Validate genre
        if not genre:
            print(f"ERROR: Genre required. Use --genre <type>")
            print(f"Valid genres: {', '.join(VALID_GENRES)}")
            return False
        genre = genre.lower()
        if genre not in VALID_GENRES:
            print(f"ERROR: Invalid genre '{genre}'")
            print(f"Valid genres: {', '.join(VALID_GENRES)}")
            return False

        # Validate tags (at least one subject tag required)
        if not tags or len(tags) == 0:
            print(f"ERROR: At least one subject tag required. Use --tags tag1,tag2")
            existing = self._get_existing_tags()
            if existing:
                print(f"Existing tags: {', '.join(existing[:20])}")
            print("(New tags are created automatically)")
            return False

        # Check for potential duplicates
        clean_tags, suggestions = self.validate_tags(tags)
        if suggestions:
            print("⚠️  Similar tags already exist:")
            for new_tag, similar in suggestions.items():
                print(f"   '{new_tag}' → did you mean: {', '.join(similar)}?")
            print("   (Proceeding with your tags. Use existing tags to avoid duplication.)")

        conn = self.get_connection()
        now = datetime.now().isoformat()

        if not title:
            full_path = DOCS_ROOT / path
            if full_path.exists():
                with open(full_path) as f:
                    for line in f:
                        if line.startswith("# "):
                            title = line[2:].strip()
                            break

        conn.execute("""
            INSERT INTO documents (path, title, genre, created_at, updated_at, accessed_at)
            VALUES (?, ?, ?, ?, ?, ?)
            ON CONFLICT(path) DO UPDATE SET
                updated_at = ?,
                title = COALESCE(?, title),
                genre = COALESCE(?, genre)
        """, (path, title, genre, now, now, now, now, title, genre))

        doc_id = conn.execute(
            "SELECT id FROM documents WHERE path = ?", (path,)
        ).fetchone()[0]

        # Create tags on-demand (free-form)
        for tag in tags:
            tag = tag.strip().lower()
            conn.execute(
                "INSERT OR IGNORE INTO tags (name, created_at) VALUES (?, ?)",
                (tag, now)
            )
            conn.execute("""
                INSERT OR IGNORE INTO document_tags (document_id, tag_id)
                SELECT ?, id FROM tags WHERE name = ?
            """, (doc_id, tag))

        conn.commit()
        conn.close()
        print(f"Registered: {path}")
        print(f"  Genre: {genre}")
        print(f"  Tags: {', '.join(tags)}")
        return True

    def _get_existing_tags(self) -> list:
        """Get list of existing tags (suggestions, not restrictions)."""
        conn = self.get_connection()
        rows = conn.execute(
            "SELECT name FROM tags ORDER BY name"
        ).fetchall()
        conn.close()
        return [row[0] for row in rows]

    def suggest_similar_tags(self, tag: str) -> list:
        """Find existing tags similar to the given tag (for deduplication)."""
        existing = self._get_existing_tags()
        tag_lower = tag.lower().strip()
        suggestions = []

        for existing_tag in existing:
            # Exact match
            if existing_tag == tag_lower:
                continue  # Already exists, no suggestion needed

            # Substring match (e.g., "ecs" matches "aws-ecs")
            if tag_lower in existing_tag or existing_tag in tag_lower:
                suggestions.append(existing_tag)
                continue

            # Common prefix (e.g., "langgraph" and "langchain")
            if len(tag_lower) >= 3 and existing_tag.startswith(tag_lower[:3]):
                suggestions.append(existing_tag)
                continue

            # Simple similarity (shared words)
            tag_parts = set(tag_lower.replace("-", " ").replace("_", " ").split())
            existing_parts = set(existing_tag.replace("-", " ").replace("_", " ").split())
            if tag_parts & existing_parts:  # Any shared words
                suggestions.append(existing_tag)

        return suggestions[:5]  # Limit to 5 suggestions

    def validate_tags(self, tags: list) -> tuple[list, dict]:
        """
        Validate tags and return (clean_tags, suggestions).
        Suggestions dict maps new tags to similar existing tags.
        """
        existing = set(self._get_existing_tags())
        clean_tags = []
        suggestions = {}

        for tag in tags:
            tag = tag.lower().strip()
            if not tag:
                continue

            clean_tags.append(tag)

            # If tag doesn't exist, check for similar ones
            if tag not in existing:
                similar = self.suggest_similar_tags(tag)
                if similar:
                    suggestions[tag] = similar

        return clean_tags, suggestions

    def is_indexed(self, path: str) -> bool:
        """Check if a document is in the index."""
        conn = self.get_connection()
        result = conn.execute("SELECT 1 FROM documents WHERE path = ?", (path,)).fetchone()
        conn.close()
        return result is not None

    def mark_read(self, path: str, quiet: bool = False):
        conn = self.get_connection()
        now = datetime.now().isoformat()
        result = conn.execute(
            "UPDATE documents SET accessed_at = ? WHERE path = ?", (now, path)
        )
        conn.commit()
        conn.close()
        if not quiet:
            if result.rowcount == 0:
                print(f"Document not in index: {path}")
            else:
                print(f"Marked as read: {path}")
        return result.rowcount > 0

    def mark_updated(self, path: str, quiet: bool = False):
        conn = self.get_connection()
        now = datetime.now().isoformat()
        result = conn.execute(
            "UPDATE documents SET updated_at = ?, accessed_at = ? WHERE path = ?",
            (now, now, path)
        )
        conn.commit()
        conn.close()
        if not quiet:
            if result.rowcount == 0:
                print(f"Document not in index: {path}")
            else:
                print(f"Marked as updated: {path}")
        return result.rowcount > 0

    def add_tags(self, path: str, tags: list):
        conn = self.get_connection()
        doc = conn.execute("SELECT id FROM documents WHERE path = ?", (path,)).fetchone()
        if not doc:
            print(f"Document not in index: {path}")
            conn.close()
            return
        doc_id = doc[0]
        for tag in tags:
            tag = tag.strip().lower()
            conn.execute("INSERT OR IGNORE INTO tags (name) VALUES (?)", (tag,))
            conn.execute("""
                INSERT OR IGNORE INTO document_tags (document_id, tag_id)
                SELECT ?, id FROM tags WHERE name = ?
            """, (doc_id, tag))
        conn.commit()
        conn.close()
        print(f"Added tags to {path}: {', '.join(tags)}")

    def query_stale(self, days: int = 30):
        conn = self.get_connection()
        rows = conn.execute("""
            SELECT path, title, updated_at,
                   CAST(julianday('now') - julianday(updated_at) AS INTEGER) as days_stale
            FROM documents
            WHERE julianday('now') - julianday(updated_at) > ?
            ORDER BY updated_at ASC
        """, (days,)).fetchall()
        conn.close()
        return rows

    def query_by_tag(self, tag: str):
        conn = self.get_connection()
        rows = conn.execute("""
            SELECT d.path, d.title, d.updated_at
            FROM documents d
            JOIN document_tags dt ON d.id = dt.document_id
            JOIN tags t ON dt.tag_id = t.id
            WHERE t.name = ?
            ORDER BY d.updated_at DESC
        """, (tag.lower(),)).fetchall()
        conn.close()
        return rows

    def query_all(self):
        conn = self.get_connection()
        rows = conn.execute("""
            SELECT d.path, d.title,
                   GROUP_CONCAT(t.name, ', ') as tags,
                   d.updated_at
            FROM documents d
            LEFT JOIN document_tags dt ON d.id = dt.document_id
            LEFT JOIN tags t ON dt.tag_id = t.id
            GROUP BY d.id
            ORDER BY d.updated_at DESC
        """).fetchall()
        conn.close()
        return rows

    def query_untracked(self):
        conn = self.get_connection()
        indexed = set(row[0] for row in conn.execute("SELECT path FROM documents").fetchall())
        conn.close()

        all_docs = set()
        for md_file in DOCS_ROOT.rglob("*.md"):
            rel_path = str(md_file.relative_to(DOCS_ROOT))
            if not any(part.startswith('.') for part in rel_path.split('/')):
                all_docs.add(rel_path)

        return sorted(all_docs - indexed)

    def stats(self):
        conn = self.get_connection()
        stats = conn.execute("""
            SELECT
                (SELECT COUNT(*) FROM documents) as total_docs,
                (SELECT COUNT(*) FROM documents WHERE julianday('now') - julianday(updated_at) > 30) as stale_docs,
                (SELECT COUNT(*) FROM documents WHERE accessed_at = created_at) as never_read,
                (SELECT COUNT(*) FROM tags) as total_tags,
                (SELECT COUNT(DISTINCT tag_id) FROM document_tags) as used_tags
        """).fetchone()
        conn.close()
        return stats

# =============================================================================
# TEMPLATES (Creation)
# =============================================================================

class DocTemplates:
    """Handles template-based document creation."""

    @staticmethod
    def get_next_adr_number() -> int:
        adr_dir = DOCS_ROOT / "adrs"
        if not adr_dir.exists():
            return 1
        existing = list(adr_dir.glob("ADR-*.md"))
        if not existing:
            return 1
        numbers = []
        for f in existing:
            match = re.match(r"ADR-(\d+)", f.name)
            if match:
                numbers.append(int(match.group(1)))
        return max(numbers) + 1 if numbers else 1

    @staticmethod
    def create(doc_type: str, name: str, description: str = "") -> tuple[str, str]:
        """Create a document from template. Returns (path, content)."""

        if doc_type not in TEMPLATES:
            raise ValueError(f"Unknown doc type: {doc_type}. Valid: {list(TEMPLATES.keys())}")

        template = TEMPLATES[doc_type]

        # Determine path based on type
        if doc_type == "adr":
            number = DocTemplates.get_next_adr_number()
            filename = f"ADR-{number:03d}-{name.lower().replace(' ', '-')}.md"
            path = f"adrs/{filename}"
            title = name
        elif doc_type == "runbook":
            filename = f"{name.lower().replace(' ', '-')}.md"
            path = f"runbooks/{filename}"
            title = name
        elif doc_type in ("overview", "gotchas", "architecture", "deep-dive"):
            # These go in shadow directories
            path = f"shadow/{name}/_{doc_type.replace('-', '_')}.md"
            title = f"{name.split('/')[-1].replace('_', ' ').title()}"
        else:
            path = f"{name}.md"
            title = name

        # Fill template
        content = template.format(
            title=title,
            name=name.split('/')[-1] if '/' in name else name,
            path=name,
            description=description or "(Add description)",
            date=datetime.now().strftime("%Y-%m-%d"),
            number=DocTemplates.get_next_adr_number() if doc_type == "adr" else "",
        )

        return path, content

# =============================================================================
# VALIDATION
# =============================================================================

class DocValidator:
    """Validates documents against conventions."""

    @staticmethod
    def detect_type(path: str, content: str) -> str:
        """Detect document type from path and content."""
        path_lower = path.lower()

        if "_overview.md" in path_lower:
            return "overview"
        elif "_gotchas.md" in path_lower:
            return "gotchas"
        elif "_architecture.md" in path_lower:
            return "architecture"
        elif "_deep_dive.md" in path_lower:
            return "deep-dive"
        elif path_lower.startswith("adrs/") or "adr-" in path_lower:
            return "adr"
        elif path_lower.startswith("runbooks/"):
            return "runbook"
        else:
            return "generic"

    @staticmethod
    def validate(path: str) -> ValidationResult:
        """Validate a single document."""
        full_path = DOCS_ROOT / path
        if not full_path.exists():
            result = ValidationResult(path=path, doc_type="unknown")
            result.issues.append(ValidationIssue(
                path=path, severity="error", rule="exists",
                message="File does not exist"
            ))
            return result

        with open(full_path) as f:
            content = f.read()
            lines = content.split('\n')

        doc_type = DocValidator.detect_type(path, content)
        result = ValidationResult(path=path, doc_type=doc_type)

        # Rule: Must have title
        if not content.strip().startswith("# "):
            result.issues.append(ValidationIssue(
                path=path, severity="error", rule="has-title",
                message="Document must start with a # heading", line=1
            ))

        # Rule: Check required sections
        if doc_type in REQUIRED_SECTIONS:
            headings = re.findall(r'^##\s+(.+)$', content, re.MULTILINE)
            for section in REQUIRED_SECTIONS[doc_type]:
                if not any(section.lower() in h.lower() for h in headings):
                    result.issues.append(ValidationIssue(
                        path=path, severity="warning", rule="required-section",
                        message=f"Missing required section: {section}"
                    ))

        # Rule: Gotchas must have numbered items
        if doc_type == "gotchas":
            if not re.search(r'^###\s+\d+\.', content, re.MULTILINE):
                result.issues.append(ValidationIssue(
                    path=path, severity="warning", rule="gotcha-numbered",
                    message="Gotchas should have numbered items (### 1. Issue Name)"
                ))

        # Rule: ADRs must have status
        if doc_type == "adr":
            if not re.search(r'\*\*Status:\*\*', content):
                result.issues.append(ValidationIssue(
                    path=path, severity="error", rule="adr-status",
                    message="ADR must have **Status:** field"
                ))

        # Rule: Runbooks must have risk level
        if doc_type == "runbook":
            if not re.search(r'\*\*Risk Level:\*\*', content, re.IGNORECASE):
                result.issues.append(ValidationIssue(
                    path=path, severity="warning", rule="runbook-risk",
                    message="Runbook should have **Risk Level:** field"
                ))

        # Rule: Check for empty sections
        for i, line in enumerate(lines):
            if line.startswith("## "):
                # Check if next non-empty line is another heading
                for j in range(i + 1, min(i + 5, len(lines))):
                    if lines[j].strip():
                        if lines[j].startswith("#"):
                            result.issues.append(ValidationIssue(
                                path=path, severity="info", rule="empty-section",
                                message=f"Section appears empty: {line.strip()}",
                                line=i + 1
                            ))
                        break

        # Rule: Line length (warning only)
        for i, line in enumerate(lines):
            if len(line) > 120 and not line.startswith("|") and not line.startswith("```"):
                result.issues.append(ValidationIssue(
                    path=path, severity="info", rule="line-length",
                    message=f"Line exceeds 120 characters ({len(line)} chars)",
                    line=i + 1
                ))

        return result

    @staticmethod
    def validate_all() -> list[ValidationResult]:
        """Validate all documents in _docs/."""
        results = []
        for md_file in DOCS_ROOT.rglob("*.md"):
            rel_path = str(md_file.relative_to(DOCS_ROOT))
            if not any(part.startswith('.') for part in rel_path.split('/')):
                results.append(DocValidator.validate(rel_path))
        return results

# =============================================================================
# LINK CHECKING
# =============================================================================

class DocLinker:
    """Handles link integrity checking."""

    @staticmethod
    def find_links(content: str) -> list[tuple[str, int]]:
        """Find all markdown links in content. Returns [(link, line_num), ...]"""
        links = []
        for i, line in enumerate(content.split('\n')):
            # Match [text](link) pattern
            for match in re.finditer(r'\[([^\]]+)\]\(([^)]+)\)', line):
                link = match.group(2)
                # Skip external links and anchors
                if not link.startswith(('http://', 'https://', '#', 'mailto:')):
                    links.append((link, i + 1))
        return links

    @staticmethod
    def check_links(path: str) -> list[ValidationIssue]:
        """Check all internal links in a document."""
        full_path = DOCS_ROOT / path
        if not full_path.exists():
            return [ValidationIssue(
                path=path, severity="error", rule="exists",
                message="File does not exist"
            )]

        issues = []
        with open(full_path) as f:
            content = f.read()

        doc_dir = full_path.parent

        for link, line_num in DocLinker.find_links(content):
            # Handle anchor links within same file
            if '#' in link:
                link = link.split('#')[0]
                if not link:  # Just an anchor
                    continue

            # Resolve relative path
            target = (doc_dir / link).resolve()

            if not target.exists():
                issues.append(ValidationIssue(
                    path=path, severity="error", rule="broken-link",
                    message=f"Broken link: {link}", line=line_num
                ))

        return issues

    @staticmethod
    def check_all_links() -> dict[str, list[ValidationIssue]]:
        """Check links in all documents."""
        results = {}
        for md_file in DOCS_ROOT.rglob("*.md"):
            rel_path = str(md_file.relative_to(DOCS_ROOT))
            if not any(part.startswith('.') for part in rel_path.split('/')):
                issues = DocLinker.check_links(rel_path)
                if issues:
                    results[rel_path] = issues
        return results

# =============================================================================
# LIFECYCLE (Archive, Migrate)
# =============================================================================

class DocLifecycle:
    """Handles document lifecycle operations."""

    @staticmethod
    def archive(path: str, reason: str = ""):
        """Mark a document as deprecated."""
        full_path = DOCS_ROOT / path
        if not full_path.exists():
            print(f"File not found: {path}")
            return False

        with open(full_path) as f:
            content = f.read()

        # Add deprecation notice after title
        lines = content.split('\n')
        for i, line in enumerate(lines):
            if line.startswith('# '):
                notice = f"\n> [!WARNING] Deprecated\n> This document is deprecated{': ' + reason if reason else '.'}.\n> Archived on {datetime.now().strftime('%Y-%m-%d')}.\n"
                lines.insert(i + 1, notice)
                break

        with open(full_path, 'w') as f:
            f.write('\n'.join(lines))

        print(f"Archived: {path}")
        return True

    @staticmethod
    def find_references(path: str) -> list[tuple[str, int]]:
        """Find all documents that link to this path."""
        refs = []
        path_variants = [path, f"./{path}", f"../{path}"]

        for md_file in DOCS_ROOT.rglob("*.md"):
            rel_path = str(md_file.relative_to(DOCS_ROOT))
            if rel_path == path:
                continue

            with open(md_file) as f:
                for i, line in enumerate(f):
                    for variant in path_variants:
                        if variant in line:
                            refs.append((rel_path, i + 1))
                            break

        return refs

    @staticmethod
    def migrate(old_path: str, new_path: str, update_refs: bool = True):
        """Move a document and optionally update references."""
        old_full = DOCS_ROOT / old_path
        new_full = DOCS_ROOT / new_path

        if not old_full.exists():
            print(f"Source not found: {old_path}")
            return False

        # Create target directory if needed
        new_full.parent.mkdir(parents=True, exist_ok=True)

        # Move file
        old_full.rename(new_full)
        print(f"Moved: {old_path} -> {new_path}")

        # Update references
        if update_refs:
            refs = DocLifecycle.find_references(old_path)
            for ref_path, _ in refs:
                ref_full = DOCS_ROOT / ref_path
                with open(ref_full) as f:
                    content = f.read()

                # Simple replacement (may need refinement for relative paths)
                updated = content.replace(old_path, new_path)

                with open(ref_full, 'w') as f:
                    f.write(updated)

                print(f"  Updated reference in: {ref_path}")

        # Update index
        index = DocIndex()
        conn = index.get_connection()
        conn.execute("UPDATE documents SET path = ? WHERE path = ?", (new_path, old_path))
        conn.commit()
        conn.close()

        return True

# =============================================================================
# REPORTS
# =============================================================================

class DocReports:
    """Generate documentation reports."""

    @staticmethod
    def freshness_report():
        """Report on document freshness."""
        index = DocIndex()
        stale = index.query_stale(30)
        very_stale = index.query_stale(90)

        print("Documentation Freshness Report")
        print("=" * 50)
        print(f"\nStale (>30 days): {len(stale)}")
        print(f"Very stale (>90 days): {len(very_stale)}")

        if very_stale:
            print("\nVery Stale Documents:")
            for row in very_stale:
                print(f"  {row['path']} ({row['days_stale']} days)")

        if stale:
            print("\nStale Documents:")
            for row in stale:
                if row['days_stale'] <= 90:
                    print(f"  {row['path']} ({row['days_stale']} days)")

    @staticmethod
    def quality_report():
        """Report on documentation quality."""
        results = DocValidator.validate_all()

        total_errors = sum(r.error_count for r in results)
        total_warnings = sum(r.warning_count for r in results)

        print("Documentation Quality Report")
        print("=" * 50)
        print(f"\nDocuments checked: {len(results)}")
        print(f"Total errors: {total_errors}")
        print(f"Total warnings: {total_warnings}")

        if total_errors > 0 or total_warnings > 0:
            print("\nIssues by document:")
            for result in results:
                if result.issues:
                    print(f"\n  {result.path} ({result.doc_type}):")
                    for issue in result.issues:
                        prefix = "❌" if issue.severity == "error" else "⚠️" if issue.severity == "warning" else "ℹ️"
                        line = f" (line {issue.line})" if issue.line else ""
                        print(f"    {prefix} {issue.message}{line}")

    @staticmethod
    def coverage_report():
        """Report on documentation coverage."""
        index = DocIndex()

        # Get tag coverage
        conn = index.get_connection()
        coverage = conn.execute("""
            SELECT t.name, COUNT(dt.document_id) as doc_count
            FROM tags t
            LEFT JOIN document_tags dt ON t.id = dt.tag_id
            GROUP BY t.id
            ORDER BY doc_count DESC
        """).fetchall()
        conn.close()

        print("Documentation Coverage Report")
        print("=" * 50)
        print("\nDocuments by Tag:")
        for row in coverage:
            bar = "█" * row['doc_count'] if row['doc_count'] else "░"
            print(f"  {row['name']:<20} {row['doc_count']:>3} {bar}")

        # Check for untracked
        untracked = index.query_untracked()
        if untracked:
            print(f"\nUntracked documents: {len(untracked)}")
            for path in untracked[:10]:
                print(f"  {path}")
            if len(untracked) > 10:
                print(f"  ... and {len(untracked) - 10} more")

# =============================================================================
# MEMORY SYSTEM (FTS5-based retrieval)
# =============================================================================

class DocMemory:
    """Handles content chunking and FTS5-based retrieval for AI collaboration."""

    def __init__(self):
        self.db_path = DB_PATH

    def get_connection(self):
        conn = sqlite3.connect(self.db_path)
        conn.execute("PRAGMA foreign_keys = ON")
        conn.row_factory = sqlite3.Row
        return conn

    def _hash_content(self, content: str) -> str:
        """Generate SHA256 hash for deduplication."""
        return hashlib.sha256(content.encode()).hexdigest()

    def _chunk_document(self, content: str, source: str) -> list[dict]:
        """
        Chunk a markdown document by headings.
        Returns list of {content, section, source}.
        """
        chunks = []
        lines = content.split('\n')

        current_section = []
        current_headings = []

        for line in lines:
            # Detect heading
            if line.startswith('#'):
                # Save previous section if it has content
                if current_section:
                    section_content = '\n'.join(current_section).strip()
                    if section_content and len(section_content) > 50:  # Skip tiny sections
                        chunks.append({
                            'content': section_content,
                            'section': ' > '.join(current_headings) if current_headings else 'Introduction',
                            'source': source,
                        })
                    current_section = []

                # Update heading stack
                level = len(line) - len(line.lstrip('#'))
                heading_text = line.lstrip('#').strip()

                # Trim heading stack to current level
                current_headings = current_headings[:level-1]
                current_headings.append(heading_text)

            current_section.append(line)

        # Don't forget the last section
        if current_section:
            section_content = '\n'.join(current_section).strip()
            if section_content and len(section_content) > 50:
                chunks.append({
                    'content': section_content,
                    'section': ' > '.join(current_headings) if current_headings else 'Content',
                    'source': source,
                })

        return chunks

    def ingest(self, path: str, tags: list = None) -> int:
        """Ingest a document into the memory system. Returns chunk count."""
        full_path = DOCS_ROOT / path
        if not full_path.exists():
            print(f"File not found: {path}")
            return 0

        with open(full_path) as f:
            content = f.read()

        chunks = self._chunk_document(content, path)

        if not chunks:
            print(f"No chunks generated from: {path}")
            return 0

        conn = self.get_connection()
        now = datetime.now().isoformat()

        # Infer tags from path if not provided
        if not tags:
            tags = []
            path_lower = path.lower()
            for tag_hint in ['infra', 'agents', 'apps', 'shared', 'pipelines', 'ecs', 'lambda']:
                if tag_hint in path_lower:
                    tags.append(tag_hint)
            if not tags:
                tags = ['general']

        tags_str = ' '.join(tags)  # Space-separated for FTS
        tags_csv = ','.join(tags)  # Comma-separated for metadata

        inserted = 0
        for chunk in chunks:
            content_hash = self._hash_content(chunk['content'])

            # Check for duplicate
            existing = conn.execute(
                "SELECT id FROM chunk_meta WHERE content_hash = ?", (content_hash,)
            ).fetchone()

            if existing:
                continue  # Skip duplicate

            # Insert into metadata table
            cursor = conn.execute("""
                INSERT INTO chunk_meta (source, section, tags, chunk_type, created_at, content_hash)
                VALUES (?, ?, ?, 'doc', ?, ?)
            """, (chunk['source'], chunk['section'], tags_csv, now, content_hash))

            # Insert into FTS table
            conn.execute("""
                INSERT INTO chunks_fts (rowid, content, source, section, tags)
                VALUES (?, ?, ?, ?, ?)
            """, (cursor.lastrowid, chunk['content'], chunk['source'], chunk['section'], tags_str))

            inserted += 1

        conn.commit()
        conn.close()

        print(f"Ingested: {path} ({inserted} chunks)")
        return inserted

    def ingest_all(self) -> int:
        """Ingest all indexed documents."""
        index = DocIndex()
        rows = index.query_all()

        total = 0
        for row in rows:
            count = self.ingest(row['path'])
            total += count

        print(f"\nTotal: {total} chunks from {len(rows)} documents")
        return total

    def add(self, content: str, tags: list, source: str = None, chunk_type: str = 'note') -> bool:
        """Add a memory/note directly."""
        if not content.strip():
            print("ERROR: Content cannot be empty")
            return False

        if not tags:
            print("ERROR: At least one tag required")
            return False

        conn = self.get_connection()
        now = datetime.now().isoformat()
        content_hash = self._hash_content(content)

        # Check for duplicate
        existing = conn.execute(
            "SELECT id FROM chunk_meta WHERE content_hash = ?", (content_hash,)
        ).fetchone()

        if existing:
            print("Note already exists (duplicate content)")
            conn.close()
            return False

        # Default source to session date
        if not source:
            source = f"session:{datetime.now().strftime('%Y-%m-%d')}"

        tags_str = ' '.join(tags)
        tags_csv = ','.join(tags)

        # Insert into metadata table
        cursor = conn.execute("""
            INSERT INTO chunk_meta (source, section, tags, chunk_type, created_at, content_hash)
            VALUES (?, ?, ?, ?, ?, ?)
        """, (source, 'Note', tags_csv, chunk_type, now, content_hash))

        # Insert into FTS table
        conn.execute("""
            INSERT INTO chunks_fts (rowid, content, source, section, tags)
            VALUES (?, ?, ?, ?, ?)
        """, (cursor.lastrowid, content, source, 'Note', tags_str))

        conn.commit()
        conn.close()

        print(f"Added {chunk_type}: {content[:50]}...")
        print(f"  Tags: {', '.join(tags)}")
        print(f"  Source: {source}")
        return True

    def search(self, query: str, tags: list = None, limit: int = 10) -> list[dict]:
        """Search memory using FTS5. Returns matching chunks with age information."""
        conn = self.get_connection()

        # Build FTS query - split words and join with OR for flexible matching
        words = query.split()
        # Escape special FTS characters and quote each word
        safe_words = [w.replace('"', '""') for w in words if w.strip()]

        if len(safe_words) > 1:
            # Multiple words: match any word (OR) for broader results
            fts_query = ' OR '.join(safe_words)
        elif safe_words:
            fts_query = safe_words[0]
        else:
            fts_query = query

        if tags:
            # Include tags in the search
            tag_filter = ' OR '.join(tags)
            fts_query = f'({fts_query}) OR tags:({tag_filter})'

        try:
            rows = conn.execute("""
                SELECT
                    c.content,
                    c.source,
                    c.section,
                    c.tags,
                    m.chunk_type,
                    m.created_at,
                    bm25(chunks_fts) as relevance
                FROM chunks_fts c
                JOIN chunk_meta m ON c.rowid = m.id
                WHERE chunks_fts MATCH ?
                ORDER BY relevance
                LIMIT ?
            """, (fts_query, limit)).fetchall()
        except sqlite3.OperationalError as e:
            # If FTS query fails, try simpler query
            rows = conn.execute("""
                SELECT
                    c.content,
                    c.source,
                    c.section,
                    c.tags,
                    m.chunk_type,
                    m.created_at,
                    0 as relevance
                FROM chunks_fts c
                JOIN chunk_meta m ON c.rowid = m.id
                WHERE c.content LIKE ? OR c.tags LIKE ?
                ORDER BY m.created_at DESC
                LIMIT ?
            """, (f'%{query}%', f'%{query}%', limit)).fetchall()

        conn.close()

        results = []
        now = datetime.now()
        for row in rows:
            # Calculate age in days
            created_at = row['created_at']
            try:
                created = datetime.fromisoformat(created_at)
                age_days = (now - created).days
            except (ValueError, TypeError):
                age_days = 0

            # Staleness threshold: >90 days
            is_stale = age_days > 90

            results.append({
                'content': row['content'],
                'source': row['source'],
                'section': row['section'],
                'tags': row['tags'],
                'type': row['chunk_type'],
                'created_at': created_at,
                'age_days': age_days,
                'is_stale': is_stale,
            })

        return results

    def stats(self) -> dict:
        """Get memory system statistics."""
        conn = self.get_connection()

        stats = {}

        # Total chunks
        result = conn.execute("SELECT COUNT(*) as count FROM chunk_meta").fetchone()
        stats['total_chunks'] = result['count'] if result else 0

        # By type
        rows = conn.execute("""
            SELECT chunk_type, COUNT(*) as count
            FROM chunk_meta
            GROUP BY chunk_type
        """).fetchall()
        stats['by_type'] = {row['chunk_type']: row['count'] for row in rows}

        # By source (top 10)
        rows = conn.execute("""
            SELECT source, COUNT(*) as count
            FROM chunk_meta
            GROUP BY source
            ORDER BY count DESC
            LIMIT 10
        """).fetchall()
        stats['top_sources'] = [(row['source'], row['count']) for row in rows]

        # Unique sources
        result = conn.execute("SELECT COUNT(DISTINCT source) as count FROM chunk_meta").fetchone()
        stats['unique_sources'] = result['count'] if result else 0

        conn.close()
        return stats

    def recent(self, days: int = 7, limit: int = 10) -> list[dict]:
        """Get recently added memories with age information."""
        conn = self.get_connection()

        rows = conn.execute("""
            SELECT c.content, c.source, c.section, c.tags,
                   m.chunk_type, m.created_at
            FROM chunks_fts c
            JOIN chunk_meta m ON c.rowid = m.id
            WHERE date(m.created_at) >= date('now', ?)
            ORDER BY m.created_at DESC
            LIMIT ?
        """, (f'-{days} days', limit)).fetchall()

        conn.close()

        results = []
        now = datetime.now()
        for row in rows:
            # Calculate age in days
            created_at = row['created_at']
            try:
                created = datetime.fromisoformat(created_at)
                age_days = (now - created).days
            except (ValueError, TypeError):
                age_days = 0

            # Staleness threshold: >90 days
            is_stale = age_days > 90

            results.append({
                'content': row['content'],
                'source': row['source'],
                'section': row['section'],
                'tags': row['tags'],
                'type': row['chunk_type'],
                'created_at': created_at,
                'age_days': age_days,
                'is_stale': is_stale,
            })

        return results

    def refresh(self, path: str = None) -> dict:
        """
        Refresh doc chunks from source files.
        Deletes stale chunks, re-ingests current content.
        Preserves notes/decisions (chunk_type != 'doc').

        Args:
            path: Optional path to refresh (relative to _docs/).
                  If None, refreshes all doc-type chunks.

        Returns:
            Dictionary with deleted, ingested, and sources counts.
        """
        conn = self.get_connection()

        if path:
            # Single document refresh
            sources = [path]
        else:
            # All docs - get unique sources where type='doc'
            rows = conn.execute(
                "SELECT DISTINCT source FROM chunk_meta WHERE chunk_type = 'doc'"
            ).fetchall()
            sources = [row['source'] for row in rows]

        deleted = 0
        ingested = 0

        for source in sources:
            # Delete existing doc chunks for this source
            chunks = conn.execute(
                "SELECT id FROM chunk_meta WHERE source = ? AND chunk_type = 'doc'",
                (source,)
            ).fetchall()

            for chunk in chunks:
                conn.execute("DELETE FROM chunks_fts WHERE rowid = ?", (chunk['id'],))
                conn.execute("DELETE FROM chunk_meta WHERE id = ?", (chunk['id'],))
                deleted += 1

            conn.commit()

            # Re-ingest if file exists
            full_path = DOCS_ROOT / source
            if full_path.exists():
                # Close connection before ingesting (ingest opens its own)
                conn.close()
                ingested += self.ingest(source)
                conn = self.get_connection()

        conn.close()
        return {'deleted': deleted, 'ingested': ingested, 'sources': len(sources)}


# =============================================================================
# CLI
# =============================================================================

def print_usage():
    print(__doc__)

def main():
    if len(sys.argv) < 2:
        print_usage()
        sys.exit(1)

    cmd = sys.argv[1]

    # === CREATE ===
    if cmd == "create":
        if len(sys.argv) < 4:
            print("Usage: doctool create <type> <name> [--tags t1,t2]")
            print(f"Types: {', '.join(TEMPLATES.keys())}")
            sys.exit(1)

        doc_type = sys.argv[2]
        name = sys.argv[3]
        tags = None
        if "--tags" in sys.argv:
            idx = sys.argv.index("--tags")
            if idx + 1 < len(sys.argv):
                tags = sys.argv[idx + 1].split(",")

        path, content = DocTemplates.create(doc_type, name)
        full_path = DOCS_ROOT / path

        # Create parent directories
        full_path.parent.mkdir(parents=True, exist_ok=True)

        # Write file
        with open(full_path, 'w') as f:
            f.write(content)

        print(f"Created: {path}")

        # Validate
        result = DocValidator.validate(path)
        if result.issues:
            print(f"\nValidation ({result.error_count} errors, {result.warning_count} warnings):")
            for issue in result.issues:
                prefix = "❌" if issue.severity == "error" else "⚠️" if issue.severity == "warning" else "ℹ️"
                print(f"  {prefix} {issue.message}")

        # Register in index with genre + inferred tags
        index = DocIndex()

        # Genre comes from doc_type (template type = genre)
        genre = doc_type if doc_type in VALID_GENRES else "reference"

        # Infer subject tags from path
        inferred_tags = []
        if "shadow/infra" in path or "infra/" in path:
            inferred_tags.append("infra")
        if "shadow/agents" in path or "agents/" in path:
            inferred_tags.append("agents")
        if "shadow/apps" in path or "apps/" in path:
            inferred_tags.append("apps")
        if "shadow/shared" in path or "shared/" in path:
            inferred_tags.append("shared")
        if "shadow/pipelines" in path or "pipelines/" in path:
            inferred_tags.append("pipelines")

        all_tags = list(set((tags or []) + inferred_tags))

        # Ensure at least one tag
        if not all_tags:
            all_tags = ["untagged"]

        index.register(path, genre, all_tags)

    # === VALIDATE ===
    elif cmd == "validate":
        if len(sys.argv) < 3:
            print("Usage: doctool validate <path> | --all")
            sys.exit(1)

        index = DocIndex()

        if sys.argv[2] == "--all":
            results = DocValidator.validate_all()
            total_errors = sum(r.error_count for r in results)
            total_warnings = sum(r.warning_count for r in results)

            # Auto-update accessed_at for all validated docs that are indexed
            for result in results:
                if index.is_indexed(result.path):
                    index.mark_read(result.path, quiet=True)

            print(f"Validated {len(results)} documents")
            print(f"Errors: {total_errors}, Warnings: {total_warnings}")

            for result in results:
                if result.issues:
                    print(f"\n{result.path}:")
                    for issue in result.issues:
                        prefix = "❌" if issue.severity == "error" else "⚠️" if issue.severity == "warning" else "ℹ️"
                        line = f" (line {issue.line})" if issue.line else ""
                        print(f"  {prefix} {issue.message}{line}")
        else:
            path = sys.argv[2]
            result = DocValidator.validate(path)

            # Auto-update accessed_at if indexed
            if index.is_indexed(path):
                index.mark_read(path)

            if result.issues:
                print(f"Validation issues for {path} ({result.doc_type}):")
                for issue in result.issues:
                    prefix = "❌" if issue.severity == "error" else "⚠️" if issue.severity == "warning" else "ℹ️"
                    line = f" (line {issue.line})" if issue.line else ""
                    print(f"  {prefix} {issue.message}{line}")
            else:
                print(f"✅ {path} is valid")

    # === LINT ===
    elif cmd == "lint":
        results = DocValidator.validate_all()
        link_issues = DocLinker.check_all_links()

        total_errors = sum(r.error_count for r in results)
        total_warnings = sum(r.warning_count for r in results)
        total_broken_links = sum(len(issues) for issues in link_issues.values())

        print("Documentation Lint Report")
        print("=" * 50)
        print(f"Documents: {len(results)}")
        print(f"Errors: {total_errors}")
        print(f"Warnings: {total_warnings}")
        print(f"Broken links: {total_broken_links}")

        if total_errors > 0 or total_warnings > 0:
            print("\nValidation Issues:")
            for result in results:
                if result.issues:
                    print(f"\n  {result.path}:")
                    for issue in result.issues:
                        if issue.severity in ("error", "warning"):
                            prefix = "❌" if issue.severity == "error" else "⚠️"
                            print(f"    {prefix} {issue.message}")

        if link_issues:
            print("\nBroken Links:")
            for path, issues in link_issues.items():
                print(f"\n  {path}:")
                for issue in issues:
                    print(f"    ❌ {issue.message} (line {issue.line})")

    # === LINKS ===
    elif cmd == "links":
        if len(sys.argv) < 3:
            print("Usage: doctool links [check|report]")
            sys.exit(1)

        subcmd = sys.argv[2]
        if subcmd == "check":
            issues = DocLinker.check_all_links()
            if issues:
                print(f"Found {sum(len(i) for i in issues.values())} broken links:")
                for path, path_issues in issues.items():
                    print(f"\n  {path}:")
                    for issue in path_issues:
                        print(f"    Line {issue.line}: {issue.message}")
            else:
                print("✅ All internal links are valid")

        elif subcmd == "report":
            # Count all links
            total_links = 0
            broken = 0
            by_doc = {}

            for md_file in DOCS_ROOT.rglob("*.md"):
                rel_path = str(md_file.relative_to(DOCS_ROOT))
                if any(part.startswith('.') for part in rel_path.split('/')):
                    continue

                with open(md_file) as f:
                    content = f.read()

                links = DocLinker.find_links(content)
                total_links += len(links)

                issues = DocLinker.check_links(rel_path)
                broken += len(issues)
                if links:
                    by_doc[rel_path] = {"total": len(links), "broken": len(issues)}

            print("Link Integrity Report")
            print("=" * 50)
            print(f"Total internal links: {total_links}")
            print(f"Broken links: {broken}")
            print(f"Health: {((total_links - broken) / total_links * 100) if total_links else 100:.1f}%")

    # === ARCHIVE ===
    elif cmd == "archive":
        if len(sys.argv) < 3:
            print("Usage: doctool archive <path> [reason]")
            sys.exit(1)

        path = sys.argv[2]
        reason = " ".join(sys.argv[3:]) if len(sys.argv) > 3 else ""
        DocLifecycle.archive(path, reason)

    # === MIGRATE ===
    elif cmd == "migrate":
        if len(sys.argv) < 4:
            print("Usage: doctool migrate <old_path> <new_path>")
            sys.exit(1)

        old_path = sys.argv[2]
        new_path = sys.argv[3]
        DocLifecycle.migrate(old_path, new_path)

    # === INDEX (existing functionality) ===
    elif cmd == "index":
        if len(sys.argv) < 3:
            print("Usage: doctool index [init|register|read|update|tag|query|stats]")
            sys.exit(1)

        index = DocIndex()
        subcmd = sys.argv[2]

        if subcmd == "init":
            index.init_db()

        elif subcmd == "register":
            if len(sys.argv) < 4:
                print("Usage: doctool index register <path> --genre <type> --tags t1,t2")
                print("       Genre (controlled): " + ", ".join(VALID_GENRES))
                print("       Tags (free-form): at least one required, new tags created automatically")
                sys.exit(1)

            path = sys.argv[3]

            # Parse genre
            genre = None
            if "--genre" in sys.argv:
                idx = sys.argv.index("--genre")
                if idx + 1 < len(sys.argv):
                    genre = sys.argv[idx + 1]

            # Parse tags
            tags = None
            if "--tags" in sys.argv:
                idx = sys.argv.index("--tags")
                if idx + 1 < len(sys.argv):
                    tags = sys.argv[idx + 1].split(",")

            if not index.register(path, genre, tags):
                sys.exit(1)

        elif subcmd == "read":
            if len(sys.argv) < 4:
                print("Usage: doctool index read <path>")
                sys.exit(1)
            index.mark_read(sys.argv[3])

        elif subcmd == "update":
            if len(sys.argv) < 4:
                print("Usage: doctool index update <path>")
                sys.exit(1)
            index.mark_updated(sys.argv[3])

        elif subcmd == "tag":
            if len(sys.argv) < 5:
                print("Usage: doctool index tag <path> <tag1,tag2,...>")
                sys.exit(1)
            index.add_tags(sys.argv[3], sys.argv[4].split(","))

        elif subcmd == "query":
            if len(sys.argv) < 4:
                print("Usage: doctool index query [stale|tag|all|untracked]")
                sys.exit(1)

            query_type = sys.argv[3]

            if query_type == "stale":
                days = 30
                if "--days" in sys.argv:
                    idx = sys.argv.index("--days")
                    if idx + 1 < len(sys.argv):
                        days = int(sys.argv[idx + 1])
                rows = index.query_stale(days)
                if rows:
                    print(f"Stale documents (>{days} days):\n")
                    for row in rows:
                        print(f"  {row['path']} ({row['days_stale']} days)")
                else:
                    print(f"No stale documents (>{days} days)")

            elif query_type == "tag":
                if len(sys.argv) < 5:
                    print("Usage: doctool index query tag <tagname>")
                    sys.exit(1)
                rows = index.query_by_tag(sys.argv[4])
                if rows:
                    print(f"Documents tagged '{sys.argv[4]}':\n")
                    for row in rows:
                        print(f"  {row['path']}")
                else:
                    print(f"No documents with tag: {sys.argv[4]}")

            elif query_type == "all":
                rows = index.query_all()
                if rows:
                    print(f"{'Path':<45} {'Tags':<25} {'Updated':<12}")
                    print("-" * 85)
                    for row in rows:
                        tags = (row['tags'] or '')[:23]
                        print(f"{row['path']:<45} {tags:<25} {row['updated_at'][:10]:<12}")
                else:
                    print("No documents in index")

            elif query_type == "untracked":
                untracked = index.query_untracked()
                if untracked:
                    print(f"Untracked documents ({len(untracked)}):\n")
                    for path in untracked:
                        print(f"  {path}")
                else:
                    print("All documents are indexed")

        elif subcmd == "stats":
            stats = index.stats()
            print("Documentation Index Statistics")
            print("=" * 40)
            print(f"Total documents:     {stats['total_docs']}")
            print(f"Stale (>30 days):    {stats['stale_docs']}")
            print(f"Never accessed:      {stats['never_read']}")
            print(f"Total tags:          {stats['total_tags']}")
            print(f"Tags in use:         {stats['used_tags']}")

        elif subcmd == "tags":
            # Subcommand for managing subject tags
            if len(sys.argv) < 4:
                print("Usage: doctool index tags [list|add|search]")
                sys.exit(1)

            tags_cmd = sys.argv[3]

            if tags_cmd == "list":
                conn = index.get_connection()
                rows = conn.execute("""
                    SELECT t.name, COUNT(dt.document_id) as doc_count, t.created_at
                    FROM tags t
                    LEFT JOIN document_tags dt ON t.id = dt.tag_id
                    GROUP BY t.id
                    ORDER BY doc_count DESC, t.name
                """).fetchall()
                conn.close()

                print(f"{'Tag':<25} {'Docs':<6} {'Created':<12}")
                print("-" * 45)
                for row in rows:
                    created = row['created_at'][:10] if row['created_at'] else 'seeded'
                    print(f"{row['name']:<25} {row['doc_count']:<6} {created:<12}")

            elif tags_cmd == "add":
                if len(sys.argv) < 5:
                    print("Usage: doctool index tags add <tag1,tag2,...>")
                    sys.exit(1)
                new_tags = sys.argv[4].split(",")
                conn = index.get_connection()
                now = datetime.now().isoformat()
                for tag in new_tags:
                    tag = tag.strip().lower()
                    # Check for similar existing tags
                    similar = index.suggest_similar_tags(tag)
                    if similar:
                        print(f"⚠️  '{tag}' similar to existing: {', '.join(similar)}")
                    conn.execute(
                        "INSERT OR IGNORE INTO tags (name, created_at) VALUES (?, ?)",
                        (tag, now)
                    )
                    print(f"Added tag: {tag}")
                conn.commit()
                conn.close()

            elif tags_cmd == "search":
                if len(sys.argv) < 5:
                    print("Usage: doctool index tags search <partial>")
                    sys.exit(1)
                partial = sys.argv[4].lower()
                conn = index.get_connection()
                rows = conn.execute(
                    "SELECT name FROM tags WHERE name LIKE ? ORDER BY name",
                    (f"%{partial}%",)
                ).fetchall()
                conn.close()
                if rows:
                    print(f"Tags matching '{partial}':")
                    for row in rows:
                        print(f"  {row['name']}")
                else:
                    print(f"No tags matching '{partial}'")

            else:
                print(f"Unknown tags command: {tags_cmd}")

    # === REPORT ===
    elif cmd == "report":
        if len(sys.argv) < 3:
            print("Usage: doctool report [freshness|quality|coverage]")
            sys.exit(1)

        report_type = sys.argv[2]

        if report_type == "freshness":
            DocReports.freshness_report()
        elif report_type == "quality":
            DocReports.quality_report()
        elif report_type == "coverage":
            DocReports.coverage_report()
        else:
            print(f"Unknown report type: {report_type}")

    # === MEMORY ===
    elif cmd == "memory":
        if len(sys.argv) < 3:
            print("Usage: doctool memory [ingest|refresh|add|search|stats]")
            sys.exit(1)

        memory = DocMemory()
        subcmd = sys.argv[2]

        if subcmd == "ingest":
            # Ingest new documents - use 'refresh' for updating existing docs
            if len(sys.argv) < 4:
                print("Usage: doctool memory ingest <path> | --all")
                print("       Use 'memory refresh' to update existing documents")
                sys.exit(1)

            if sys.argv[3] == "--all":
                memory.ingest_all()
            else:
                path = sys.argv[3]
                tags = None
                if "--tags" in sys.argv:
                    idx = sys.argv.index("--tags")
                    if idx + 1 < len(sys.argv):
                        tags = sys.argv[idx + 1].split(",")
                memory.ingest(path, tags)

        elif subcmd == "refresh":
            # Refresh doc chunks from source files
            if len(sys.argv) < 4 and "--all" not in sys.argv:
                print("Usage: doctool memory refresh <path> | --all")
                print("       <path>  - Refresh a single document (relative to _docs/)")
                print("       --all   - Rebuild all doc chunks from files")
                sys.exit(1)

            if "--all" in sys.argv:
                result = memory.refresh()
            else:
                result = memory.refresh(sys.argv[3])

            print(f"Refreshed {result['sources']} source(s)")
            print(f"  Deleted: {result['deleted']} stale chunks")
            print(f"  Ingested: {result['ingested']} fresh chunks")

        elif subcmd == "add":
            # Content is everything between 'add' and '--tags' (or end)
            content_parts = []
            i = 3
            while i < len(sys.argv) and not sys.argv[i].startswith('--'):
                content_parts.append(sys.argv[i])
                i += 1
            content = ' '.join(content_parts)

            tags = None
            if "--tags" in sys.argv:
                idx = sys.argv.index("--tags")
                if idx + 1 < len(sys.argv):
                    tags = sys.argv[idx + 1].split(",")

            chunk_type = "note"
            if "--type" in sys.argv:
                idx = sys.argv.index("--type")
                if idx + 1 < len(sys.argv):
                    chunk_type = sys.argv[idx + 1]

            source = None
            if "--source" in sys.argv:
                idx = sys.argv.index("--source")
                if idx + 1 < len(sys.argv):
                    source = sys.argv[idx + 1]

            # If no inline content, read from stdin
            if not content.strip():
                if not sys.stdin.isatty():
                    # Piped input
                    content = sys.stdin.read().strip()
                else:
                    # Interactive mode
                    print("Enter content (Ctrl+D to finish):")
                    content = sys.stdin.read().strip()

            if not content.strip():
                print("ERROR: Content cannot be empty")
                print("Usage: doctool memory add <content> --tags t1,t2")
                print("   or: echo 'content' | doctool memory add --tags t1,t2")
                sys.exit(1)

            if not tags:
                print("ERROR: Tags required. Use --tags t1,t2")
                sys.exit(1)

            memory.add(content, tags, source, chunk_type)

        elif subcmd == "search":
            if len(sys.argv) < 4:
                print("Usage: doctool memory search <query> [--tags t1,t2] [--limit N]")
                sys.exit(1)

            # Query is everything between 'search' and '--' flags
            query_parts = []
            i = 3
            while i < len(sys.argv) and not sys.argv[i].startswith('--'):
                query_parts.append(sys.argv[i])
                i += 1
            query = ' '.join(query_parts)

            tags = None
            if "--tags" in sys.argv:
                idx = sys.argv.index("--tags")
                if idx + 1 < len(sys.argv):
                    tags = sys.argv[idx + 1].split(",")

            limit = 10
            if "--limit" in sys.argv:
                idx = sys.argv.index("--limit")
                if idx + 1 < len(sys.argv):
                    limit = int(sys.argv[idx + 1])

            results = memory.search(query, tags, limit)

            if results:
                print(f"Found {len(results)} results for '{query}':\n")
                for i, r in enumerate(results, 1):
                    # Format stale warning in header
                    age_days = r.get('age_days', 0)
                    is_stale = r.get('is_stale', False)
                    stale_warning = f" [STALE: {age_days} days]" if is_stale else ""
                    print(f"─── Result {i}{stale_warning} ───")

                    # Format source with age
                    age_str = f"{age_days} day{'s' if age_days != 1 else ''} ago"
                    print(f"Source: {r['source']} ({age_str})")
                    print(f"Section: {r['section']}")
                    print(f"Tags: {r['tags']}")
                    print(f"Type: {r['type']}")
                    # Truncate content for display
                    content = r['content']
                    if len(content) > 300:
                        content = content[:300] + "..."
                    print(f"\n{content}\n")
            else:
                print(f"No results for '{query}'")

        elif subcmd == "stats":
            stats = memory.stats()
            print("Memory System Statistics")
            print("=" * 40)
            print(f"Total chunks:        {stats['total_chunks']}")
            print(f"Unique sources:      {stats['unique_sources']}")
            print("\nBy type:")
            for chunk_type, count in stats.get('by_type', {}).items():
                print(f"  {chunk_type:<15} {count}")
            print("\nTop sources:")
            for source, count in stats.get('top_sources', []):
                print(f"  {source:<35} {count}")

        elif subcmd == "recent":
            days = 7
            limit = 10
            if "--days" in sys.argv:
                idx = sys.argv.index("--days")
                if idx + 1 < len(sys.argv):
                    days = int(sys.argv[idx + 1])
            if "--limit" in sys.argv:
                idx = sys.argv.index("--limit")
                if idx + 1 < len(sys.argv):
                    limit = int(sys.argv[idx + 1])

            results = memory.recent(days, limit)
            if results:
                print(f"Recent memories (last {days} days):\n")
                for r in results:
                    age_days = r.get('age_days', 0)
                    age_str = f"{age_days}d ago"
                    preview = r['content'][:60].replace('\n', ' ')
                    print(f"[{r['created_at'][:10]}] ({age_str}) {r['type']} - {preview}...")
                    print(f"  Tags: {r['tags']}\n")
            else:
                print(f"No memories in the last {days} days")

        else:
            print(f"Unknown memory command: {subcmd}")
            sys.exit(1)

    else:
        print(f"Unknown command: {cmd}")
        print_usage()
        sys.exit(1)


if __name__ == "__main__":
    main()
