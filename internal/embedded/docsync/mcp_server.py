#!/usr/bin/env python3
"""
Doc-Sync MCP Server — JSON-RPC 2.0 over stdio

Exposes documentation tools to Claude Code via Model Context Protocol.
No external dependencies (stdlib only).

Usage:
    python3 mcp_server.py  # Runs as stdio server

Protocol: JSON-RPC 2.0
Transport: stdio (read JSON from stdin, write JSON to stdout)
"""

import json
import sys
import sqlite3
import os
import re
import logging
from datetime import datetime
from pathlib import Path
from typing import Any, Optional

# =============================================================================
# CONFIGURATION
# =============================================================================

DB_PATH = Path("/workspace/_docs/.doc-index.db")
DOCS_ROOT = Path("/workspace/_docs")
SCHEMA_PATH = Path(os.path.expanduser("~/.claude/skills/doc-sync/schema.sql"))

# Controlled vocabulary for document genres
VALID_GENRES = [
    "overview", "gotchas", "architecture", "deep-dive",
    "adr", "runbook", "rfc", "guide", "reference"
]

# Logging to stderr (not stdout — that's for JSON-RPC)
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    stream=sys.stderr
)
log = logging.getLogger("doc-sync-mcp")

# =============================================================================
# DATABASE ACCESS
# =============================================================================

def get_connection():
    """Get a database connection."""
    if not DB_PATH.exists():
        raise RuntimeError(f"Database not found: {DB_PATH}. Run 'doctool index init' first.")
    conn = sqlite3.connect(DB_PATH)
    conn.execute("PRAGMA foreign_keys = ON")
    conn.row_factory = sqlite3.Row
    return conn

# =============================================================================
# TOOL IMPLEMENTATIONS
# =============================================================================

def tool_doc_search(tags: list[str] = None, genre: str = None, text: str = None) -> dict:
    """
    Search documentation by tags, genre, and/or text content.

    Args:
        tags: List of subject tags to filter by (matches any)
        genre: Document genre to filter by (overview, adr, runbook, etc.)
        text: Text to search for in document content

    Returns:
        Dictionary with matching documents
    """
    conn = get_connection()
    results = []

    try:
        if tags or genre:
            # Build query based on filters
            query = """
                SELECT DISTINCT d.path, d.title, d.genre, d.updated_at,
                       GROUP_CONCAT(t.name, ', ') as matched_tags
                FROM documents d
                LEFT JOIN document_tags dt ON d.id = dt.document_id
                LEFT JOIN tags t ON dt.tag_id = t.id
                WHERE 1=1
            """
            params = []

            if genre:
                query += " AND d.genre = ?"
                params.append(genre.lower())

            if tags:
                placeholders = ",".join("?" * len(tags))
                query += f" AND t.name IN ({placeholders})"
                params.extend([tag.lower() for tag in tags])

            query += " GROUP BY d.id ORDER BY d.updated_at DESC"
            rows = conn.execute(query, params).fetchall()

            for row in rows:
                results.append({
                    "path": row["path"],
                    "title": row["title"],
                    "genre": row["genre"],
                    "tags": row["matched_tags"],
                    "updated_at": row["updated_at"]
                })

        if text:
            # Full-text search in file content
            text_lower = text.lower()
            for md_file in DOCS_ROOT.rglob("*.md"):
                rel_path = str(md_file.relative_to(DOCS_ROOT))
                if any(part.startswith('.') for part in rel_path.split('/')):
                    continue

                try:
                    content = md_file.read_text()
                    if text_lower in content.lower():
                        # Find the matching line for context
                        for i, line in enumerate(content.split('\n')):
                            if text_lower in line.lower():
                                results.append({
                                    "path": rel_path,
                                    "line": i + 1,
                                    "context": line.strip()[:100]
                                })
                                break
                except Exception:
                    pass

        if not tags and not genre and not text:
            # Return all indexed documents
            rows = conn.execute("""
                SELECT d.path, d.title, d.genre, d.updated_at,
                       GROUP_CONCAT(t.name, ', ') as tags
                FROM documents d
                LEFT JOIN document_tags dt ON d.id = dt.document_id
                LEFT JOIN tags t ON dt.tag_id = t.id
                GROUP BY d.id
                ORDER BY d.updated_at DESC
                LIMIT 50
            """).fetchall()

            for row in rows:
                results.append({
                    "path": row["path"],
                    "title": row["title"],
                    "genre": row["genre"],
                    "tags": row["tags"],
                    "updated_at": row["updated_at"]
                })

    finally:
        conn.close()

    return {"count": len(results), "documents": results}


def tool_doc_stats() -> dict:
    """
    Get documentation index statistics.

    Returns:
        Dictionary with index statistics
    """
    conn = get_connection()

    try:
        stats = conn.execute("""
            SELECT
                (SELECT COUNT(*) FROM documents) as total_docs,
                (SELECT COUNT(*) FROM documents WHERE julianday('now') - julianday(updated_at) > 30) as stale_docs,
                (SELECT COUNT(*) FROM documents WHERE accessed_at = created_at) as never_read,
                (SELECT COUNT(*) FROM tags) as total_tags,
                (SELECT COUNT(DISTINCT tag_id) FROM document_tags) as used_tags
        """).fetchone()

        # Tag breakdown
        tag_counts = conn.execute("""
            SELECT t.name, COUNT(dt.document_id) as count
            FROM tags t
            LEFT JOIN document_tags dt ON t.id = dt.tag_id
            GROUP BY t.id
            ORDER BY count DESC
        """).fetchall()

        return {
            "total_documents": stats["total_docs"],
            "stale_documents": stats["stale_docs"],
            "never_accessed": stats["never_read"],
            "total_tags": stats["total_tags"],
            "tags_in_use": stats["used_tags"],
            "tag_breakdown": {row["name"]: row["count"] for row in tag_counts}
        }

    finally:
        conn.close()


def tool_doc_query_stale(days: int = 30) -> dict:
    """
    Find documents not updated in N days.

    Args:
        days: Number of days threshold (default 30)

    Returns:
        Dictionary with stale documents
    """
    conn = get_connection()

    try:
        rows = conn.execute("""
            SELECT path, title, updated_at,
                   CAST(julianday('now') - julianday(updated_at) AS INTEGER) as days_stale
            FROM documents
            WHERE julianday('now') - julianday(updated_at) > ?
            ORDER BY updated_at ASC
        """, (days,)).fetchall()

        return {
            "threshold_days": days,
            "count": len(rows),
            "documents": [
                {"path": row["path"], "title": row["title"], "days_stale": row["days_stale"]}
                for row in rows
            ]
        }

    finally:
        conn.close()


def tool_doc_query_untracked() -> dict:
    """
    Find documents in _docs/ that are not indexed.

    Returns:
        Dictionary with untracked documents
    """
    conn = get_connection()

    try:
        indexed = set(row[0] for row in conn.execute("SELECT path FROM documents").fetchall())
    finally:
        conn.close()

    all_docs = set()
    for md_file in DOCS_ROOT.rglob("*.md"):
        rel_path = str(md_file.relative_to(DOCS_ROOT))
        if not any(part.startswith('.') for part in rel_path.split('/')):
            all_docs.add(rel_path)

    untracked = sorted(all_docs - indexed)

    return {
        "count": len(untracked),
        "documents": untracked
    }


def tool_doc_validate(path: str) -> dict:
    """
    Validate a document against conventions.

    Args:
        path: Path to document (relative to _docs/)

    Returns:
        Dictionary with validation results
    """
    full_path = DOCS_ROOT / path
    if not full_path.exists():
        return {"valid": False, "error": f"File not found: {path}"}

    content = full_path.read_text()
    lines = content.split('\n')
    issues = []

    # Detect doc type
    path_lower = path.lower()
    if "_overview.md" in path_lower:
        doc_type = "overview"
    elif "_gotchas.md" in path_lower:
        doc_type = "gotchas"
    elif "adrs/" in path_lower or "adr-" in path_lower:
        doc_type = "adr"
    elif "runbooks/" in path_lower:
        doc_type = "runbook"
    else:
        doc_type = "generic"

    # Check for title
    if not content.strip().startswith("# "):
        issues.append({"severity": "error", "rule": "has-title", "message": "Must start with # heading"})

    # Check required sections for overview
    if doc_type == "overview":
        required = ["Quick Operations", "Purpose", "Key Files", "Architecture", "Dependencies"]
        headings = re.findall(r'^##\s+(.+)$', content, re.MULTILINE)
        for section in required:
            if not any(section.lower() in h.lower() for h in headings):
                issues.append({"severity": "warning", "rule": "required-section", "message": f"Missing: {section}"})

    # Check ADR status
    if doc_type == "adr" and not re.search(r'\*\*Status:\*\*', content):
        issues.append({"severity": "error", "rule": "adr-status", "message": "ADR must have **Status:** field"})

    # Check runbook risk level
    if doc_type == "runbook" and not re.search(r'\*\*Risk Level:\*\*', content, re.IGNORECASE):
        issues.append({"severity": "warning", "rule": "runbook-risk", "message": "Should have **Risk Level:** field"})

    return {
        "path": path,
        "doc_type": doc_type,
        "valid": not any(i["severity"] == "error" for i in issues),
        "error_count": sum(1 for i in issues if i["severity"] == "error"),
        "warning_count": sum(1 for i in issues if i["severity"] == "warning"),
        "issues": issues
    }


def tool_doc_register(path: str, genre: str, tags: list[str]) -> dict:
    """
    Register a document in the index.

    Args:
        path: Path to document (relative to _docs/)
        genre: Document genre (required, controlled vocabulary)
        tags: Subject tags (required, at least one, free-form)

    Returns:
        Dictionary with registration result
    """
    # Validate genre
    if not genre:
        return {"success": False, "error": "Genre is required", "valid_genres": VALID_GENRES}
    genre = genre.lower()
    if genre not in VALID_GENRES:
        return {"success": False, "error": f"Invalid genre: {genre}", "valid_genres": VALID_GENRES}

    # Validate tags
    if not tags or len(tags) == 0:
        return {"success": False, "error": "At least one tag is required"}

    full_path = DOCS_ROOT / path
    if not full_path.exists():
        return {"success": False, "error": f"File not found: {path}"}

    conn = get_connection()
    now = datetime.now().isoformat()

    # Extract title
    title = None
    content = full_path.read_text()
    for line in content.split('\n'):
        if line.startswith("# "):
            title = line[2:].strip()
            break

    try:
        conn.execute("""
            INSERT INTO documents (path, title, genre, created_at, updated_at, accessed_at)
            VALUES (?, ?, ?, ?, ?, ?)
            ON CONFLICT(path) DO UPDATE SET
                updated_at = ?,
                title = COALESCE(?, title),
                genre = COALESCE(?, genre)
        """, (path, title, genre, now, now, now, now, title, genre))

        doc_id = conn.execute("SELECT id FROM documents WHERE path = ?", (path,)).fetchone()[0]
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
        return {"success": True, "path": path, "title": title, "genre": genre, "tags": tags}

    finally:
        conn.close()


def tool_doc_mark_updated(path: str) -> dict:
    """
    Mark a document as updated (refreshes updated_at timestamp).

    Args:
        path: Path to document (relative to _docs/)

    Returns:
        Dictionary with result
    """
    conn = get_connection()
    now = datetime.now().isoformat()

    try:
        result = conn.execute(
            "UPDATE documents SET updated_at = ?, accessed_at = ? WHERE path = ?",
            (now, now, path)
        )
        conn.commit()

        if result.rowcount == 0:
            return {"success": False, "error": f"Document not in index: {path}"}

        return {"success": True, "path": path, "updated_at": now}

    finally:
        conn.close()


def tool_doc_list_tags() -> dict:
    """
    List all subject tags with usage counts.

    Returns:
        Dictionary with tag information
    """
    conn = get_connection()

    try:
        rows = conn.execute("""
            SELECT t.name, COUNT(dt.document_id) as count, t.created_at
            FROM tags t
            LEFT JOIN document_tags dt ON t.id = dt.tag_id
            GROUP BY t.id
            ORDER BY count DESC, t.name
        """).fetchall()

        return {
            "count": len(rows),
            "tags": [
                {"name": row["name"], "doc_count": row["count"], "created_at": row["created_at"]}
                for row in rows
            ]
        }

    finally:
        conn.close()


def tool_doc_list_genres() -> dict:
    """
    List valid document genres with usage counts.

    Returns:
        Dictionary with genre information
    """
    conn = get_connection()

    try:
        rows = conn.execute("""
            SELECT genre, COUNT(*) as count
            FROM documents
            WHERE genre IS NOT NULL
            GROUP BY genre
            ORDER BY count DESC
        """).fetchall()

        usage = {row["genre"]: row["count"] for row in rows}

        return {
            "valid_genres": VALID_GENRES,
            "usage": {genre: usage.get(genre, 0) for genre in VALID_GENRES}
        }

    finally:
        conn.close()


def tool_doc_suggest_tags(partial: str) -> dict:
    """
    Find existing tags matching a partial string (for autocomplete/deduplication).

    Args:
        partial: Partial tag name to search for

    Returns:
        Dictionary with matching tags
    """
    conn = get_connection()

    try:
        rows = conn.execute(
            "SELECT name FROM tags WHERE name LIKE ? ORDER BY name",
            (f"%{partial.lower()}%",)
        ).fetchall()

        return {
            "query": partial,
            "matches": [row["name"] for row in rows]
        }

    finally:
        conn.close()


# =============================================================================
# MEMORY TOOLS (FTS5-based retrieval)
# =============================================================================

import hashlib

def _hash_content(content: str) -> str:
    """Generate SHA256 hash for deduplication."""
    return hashlib.sha256(content.encode()).hexdigest()


def tool_memory_search(query: str, tags: list[str] = None, limit: int = 10) -> dict:
    """
    Search memory chunks using FTS5 full-text search.

    Args:
        query: Search query (words are OR'd together)
        tags: Optional tags to filter by
        limit: Maximum results (default 10)

    Returns:
        Dictionary with matching chunks including age information
    """
    conn = get_connection()

    # Build FTS query - split words and join with OR
    words = query.split()
    safe_words = [w.replace('"', '""') for w in words if w.strip()]

    if len(safe_words) > 1:
        fts_query = ' OR '.join(safe_words)
    elif safe_words:
        fts_query = safe_words[0]
    else:
        fts_query = query

    if tags:
        tag_filter = ' OR '.join(tags)
        fts_query = f'({fts_query}) OR tags:({tag_filter})'

    try:
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
        except sqlite3.OperationalError:
            # Fallback to LIKE query
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
                'content': row['content'][:500] + '...' if len(row['content']) > 500 else row['content'],
                'source': row['source'],
                'section': row['section'],
                'tags': row['tags'],
                'type': row['chunk_type'],
                'created_at': created_at,
                'age_days': age_days,
                'is_stale': is_stale,
            })

        return {"query": query, "count": len(results), "results": results}

    finally:
        conn.close()


def tool_memory_add(content: str, tags: list[str], source: str = None, chunk_type: str = 'note') -> dict:
    """
    Add a memory/note directly to the memory system.

    Args:
        content: The text content to store
        tags: Subject tags (at least one required)
        source: Optional source identifier (defaults to session:YYYY-MM-DD)
        chunk_type: Type of chunk (note, decision, session)

    Returns:
        Dictionary with result
    """
    if not content.strip():
        return {"success": False, "error": "Content cannot be empty"}

    if not tags or len(tags) == 0:
        return {"success": False, "error": "At least one tag required"}

    conn = get_connection()
    now = datetime.now().isoformat()
    content_hash = _hash_content(content)

    # Check for duplicate
    existing = conn.execute(
        "SELECT id FROM chunk_meta WHERE content_hash = ?", (content_hash,)
    ).fetchone()

    if existing:
        conn.close()
        return {"success": False, "error": "Duplicate content (already exists)"}

    if not source:
        source = f"session:{datetime.now().strftime('%Y-%m-%d')}"

    tags_str = ' '.join(tags)
    tags_csv = ','.join(tags)

    try:
        cursor = conn.execute("""
            INSERT INTO chunk_meta (source, section, tags, chunk_type, created_at, content_hash)
            VALUES (?, ?, ?, ?, ?, ?)
        """, (source, 'Note', tags_csv, chunk_type, now, content_hash))

        conn.execute("""
            INSERT INTO chunks_fts (rowid, content, source, section, tags)
            VALUES (?, ?, ?, ?, ?)
        """, (cursor.lastrowid, content, source, 'Note', tags_str))

        conn.commit()

        return {
            "success": True,
            "source": source,
            "type": chunk_type,
            "tags": tags,
            "content_preview": content[:100] + '...' if len(content) > 100 else content
        }

    finally:
        conn.close()


def tool_memory_stats() -> dict:
    """
    Get memory system statistics.

    Returns:
        Dictionary with memory statistics
    """
    conn = get_connection()

    try:
        # Total chunks
        result = conn.execute("SELECT COUNT(*) as count FROM chunk_meta").fetchone()
        total_chunks = result['count'] if result else 0

        # By type
        rows = conn.execute("""
            SELECT chunk_type, COUNT(*) as count
            FROM chunk_meta
            GROUP BY chunk_type
        """).fetchall()
        by_type = {row['chunk_type']: row['count'] for row in rows}

        # Top sources
        rows = conn.execute("""
            SELECT source, COUNT(*) as count
            FROM chunk_meta
            GROUP BY source
            ORDER BY count DESC
            LIMIT 10
        """).fetchall()
        top_sources = [(row['source'], row['count']) for row in rows]

        # Unique sources
        result = conn.execute("SELECT COUNT(DISTINCT source) as count FROM chunk_meta").fetchone()
        unique_sources = result['count'] if result else 0

        return {
            "total_chunks": total_chunks,
            "unique_sources": unique_sources,
            "by_type": by_type,
            "top_sources": dict(top_sources)
        }

    finally:
        conn.close()


def tool_memory_recent(days: int = 7, limit: int = 10) -> list[dict]:
    """
    Get recently added memories with age information.

    Args:
        days: Number of days to look back (default: 7)
        limit: Maximum number of results (default: 10)

    Returns:
        List of recent memory entries with age information
    """
    conn = get_connection()

    try:
        rows = conn.execute("""
            SELECT c.content, c.source, c.section, c.tags,
                   m.chunk_type, m.created_at
            FROM chunks_fts c
            JOIN chunk_meta m ON c.rowid = m.id
            WHERE date(m.created_at) >= date('now', ?)
            ORDER BY m.created_at DESC
            LIMIT ?
        """, (f'-{days} days', limit)).fetchall()

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

            preview = row['content'][:200].replace('\n', ' ')
            results.append({
                'date': created_at[:10],
                'type': row['chunk_type'],
                'tags': row['tags'],
                'source': row['source'],
                'preview': preview + ('...' if len(row['content']) > 200 else ''),
                'age_days': age_days,
                'is_stale': is_stale,
            })

        return results

    finally:
        conn.close()


def _chunk_document(content: str, source: str) -> list[dict]:
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


def _ingest_document(path: str, tags: list = None) -> int:
    """Ingest a single document into memory. Returns chunk count."""
    full_path = DOCS_ROOT / path
    if not full_path.exists():
        return 0

    content = full_path.read_text()
    chunks = _chunk_document(content, path)

    if not chunks:
        return 0

    conn = get_connection()
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
    try:
        for chunk in chunks:
            content_hash = _hash_content(chunk['content'])

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
    finally:
        conn.close()

    return inserted


def tool_memory_refresh(path: str = None) -> dict:
    """
    Refresh doc chunks from source files.
    Deletes stale chunks, re-ingests current content.
    Preserves notes/decisions (chunk_type != 'doc').

    Args:
        path: Optional path to refresh (relative to _docs/).
              If None/omitted, refreshes all doc-type chunks.

    Returns:
        Dictionary with deleted, ingested, and sources counts.
    """
    conn = get_connection()

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

    try:
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

        # Close connection before re-ingesting
        conn.close()

        # Re-ingest each source if file exists
        for source in sources:
            full_path = DOCS_ROOT / source
            if full_path.exists():
                ingested += _ingest_document(source)

    except Exception:
        conn.close()
        raise

    return {'deleted': deleted, 'ingested': ingested, 'sources': len(sources)}


# =============================================================================
# TOOL REGISTRY
# =============================================================================

TOOLS = {
    "doc_search": {
        "function": tool_doc_search,
        "description": "Search documentation by genre, tags, and/or text content",
        "inputSchema": {
            "type": "object",
            "properties": {
                "genre": {
                    "type": "string",
                    "description": "Document genre to filter by (overview, adr, runbook, etc.)"
                },
                "tags": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Subject tags to filter by (matches any)"
                },
                "text": {
                    "type": "string",
                    "description": "Text to search for in content"
                }
            }
        }
    },
    "doc_stats": {
        "function": tool_doc_stats,
        "description": "Get documentation index statistics",
        "inputSchema": {
            "type": "object",
            "properties": {}
        }
    },
    "doc_query_stale": {
        "function": tool_doc_query_stale,
        "description": "Find documents not updated in N days",
        "inputSchema": {
            "type": "object",
            "properties": {
                "days": {
                    "type": "integer",
                    "description": "Days threshold (default 30)",
                    "default": 30
                }
            }
        }
    },
    "doc_query_untracked": {
        "function": tool_doc_query_untracked,
        "description": "Find documents in _docs/ not in the index",
        "inputSchema": {
            "type": "object",
            "properties": {}
        }
    },
    "doc_validate": {
        "function": tool_doc_validate,
        "description": "Validate a document against conventions",
        "inputSchema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Path to document (relative to _docs/)"
                }
            },
            "required": ["path"]
        }
    },
    "doc_register": {
        "function": tool_doc_register,
        "description": "Register a document in the index with genre and tags",
        "inputSchema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Path to document (relative to _docs/)"
                },
                "genre": {
                    "type": "string",
                    "description": "Document genre (overview, gotchas, adr, runbook, etc.)"
                },
                "tags": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Subject tags (at least one required, free-form)"
                }
            },
            "required": ["path", "genre", "tags"]
        }
    },
    "doc_mark_updated": {
        "function": tool_doc_mark_updated,
        "description": "Mark a document as updated (refresh timestamp)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Path to document (relative to _docs/)"
                }
            },
            "required": ["path"]
        }
    },
    "doc_list_tags": {
        "function": tool_doc_list_tags,
        "description": "List all subject tags with usage counts",
        "inputSchema": {
            "type": "object",
            "properties": {}
        }
    },
    "doc_list_genres": {
        "function": tool_doc_list_genres,
        "description": "List valid document genres with usage counts",
        "inputSchema": {
            "type": "object",
            "properties": {}
        }
    },
    "doc_suggest_tags": {
        "function": tool_doc_suggest_tags,
        "description": "Find existing tags matching a partial string (for autocomplete)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "partial": {
                    "type": "string",
                    "description": "Partial tag name to search for"
                }
            },
            "required": ["partial"]
        }
    },
    # Memory tools
    "memory_search": {
        "function": tool_memory_search,
        "description": "Search memory chunks using full-text search. Use this to recall past discussions, decisions, and documentation context.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Search query (words are OR'd together for broad matching)"
                },
                "tags": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Optional tags to filter results"
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum results (default 10)",
                    "default": 10
                }
            },
            "required": ["query"]
        }
    },
    "memory_add": {
        "function": tool_memory_add,
        "description": "Add a note or memory to the system. Use this to capture important discussions, decisions, or context for future retrieval.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "content": {
                    "type": "string",
                    "description": "The text content to store"
                },
                "tags": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Subject tags (at least one required)"
                },
                "source": {
                    "type": "string",
                    "description": "Source identifier (defaults to session:YYYY-MM-DD)"
                },
                "chunk_type": {
                    "type": "string",
                    "enum": ["note", "decision", "session"],
                    "description": "Type of memory (default: note)"
                }
            },
            "required": ["content", "tags"]
        }
    },
    "memory_stats": {
        "function": tool_memory_stats,
        "description": "Get memory system statistics (total chunks, sources, types)",
        "inputSchema": {
            "type": "object",
            "properties": {}
        }
    },
    "memory_recent": {
        "function": tool_memory_recent,
        "description": "Get recently added memories",
        "inputSchema": {
            "type": "object",
            "properties": {
                "days": {
                    "type": "integer",
                    "description": "Number of days to look back (default: 7)"
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of results (default: 10)"
                }
            }
        }
    },
    "memory_refresh": {
        "function": tool_memory_refresh,
        "description": "Refresh doc chunks from source files. Deletes stale chunks and re-ingests current content. Preserves notes/decisions.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Path to refresh (relative to _docs/). Omit for all docs."
                }
            }
        }
    }
}

# =============================================================================
# MCP JSON-RPC PROTOCOL
# =============================================================================

def create_response(id: Any, result: Any = None, error: dict = None) -> dict:
    """Create a JSON-RPC response."""
    resp = {"jsonrpc": "2.0", "id": id}
    if error:
        resp["error"] = error
    else:
        resp["result"] = result
    return resp


def handle_initialize(params: dict) -> dict:
    """Handle initialize request."""
    return {
        "protocolVersion": "2024-11-05",
        "capabilities": {
            "tools": {}
        },
        "serverInfo": {
            "name": "doc-sync",
            "version": "1.0.0"
        }
    }


def handle_tools_list(params: dict) -> dict:
    """Handle tools/list request."""
    tools = []
    for name, tool in TOOLS.items():
        tools.append({
            "name": name,
            "description": tool["description"],
            "inputSchema": tool["inputSchema"]
        })
    return {"tools": tools}


def handle_tools_call(params: dict) -> dict:
    """Handle tools/call request."""
    name = params.get("name")
    arguments = params.get("arguments", {})

    if name not in TOOLS:
        raise ValueError(f"Unknown tool: {name}")

    tool = TOOLS[name]
    result = tool["function"](**arguments)

    return {
        "content": [
            {
                "type": "text",
                "text": json.dumps(result, indent=2)
            }
        ]
    }


def handle_request(request: dict) -> dict:
    """Handle a JSON-RPC request."""
    method = request.get("method")
    params = request.get("params", {})
    req_id = request.get("id")

    try:
        if method == "initialize":
            result = handle_initialize(params)
        elif method == "notifications/initialized":
            return None  # No response for notifications
        elif method == "tools/list":
            result = handle_tools_list(params)
        elif method == "tools/call":
            result = handle_tools_call(params)
        elif method == "ping":
            result = {}
        else:
            return create_response(req_id, error={
                "code": -32601,
                "message": f"Method not found: {method}"
            })

        return create_response(req_id, result=result)

    except Exception as e:
        log.exception(f"Error handling {method}")
        return create_response(req_id, error={
            "code": -32603,
            "message": str(e)
        })


def run_server():
    """Run the MCP server (stdio mode)."""
    log.info("Doc-Sync MCP Server starting...")

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            request = json.loads(line)
            log.debug(f"Request: {request.get('method')}")

            response = handle_request(request)

            if response:
                print(json.dumps(response), flush=True)

        except json.JSONDecodeError as e:
            log.error(f"Invalid JSON: {e}")
            print(json.dumps(create_response(None, error={
                "code": -32700,
                "message": "Parse error"
            })), flush=True)


if __name__ == "__main__":
    run_server()
