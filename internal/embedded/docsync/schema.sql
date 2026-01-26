-- Documentation Index Schema
-- Location: _docs/.doc-index.db
-- Purpose: Track document lifecycle (creation, modification, access) and tags

-- Core document registry
CREATE TABLE IF NOT EXISTS documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT UNIQUE NOT NULL,        -- relative to _docs/
    title TEXT,                       -- extracted or inferred title
    genre TEXT,                       -- controlled: decision, runbook, overview, etc.
    created_at TEXT NOT NULL,         -- ISO 8601 timestamp
    updated_at TEXT NOT NULL,         -- ISO 8601 timestamp
    accessed_at TEXT NOT NULL         -- ISO 8601 timestamp
);

-- Subject tags (free-form topics for searchability)
CREATE TABLE IF NOT EXISTS tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,        -- e.g., 'ecs', 'redis', 'auth', 'langgraph'
    created_at TEXT                   -- when this tag was first used
);

-- Many-to-many: documents <-> tags
CREATE TABLE IF NOT EXISTS document_tags (
    document_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (document_id, tag_id),
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_documents_path ON documents(path);
CREATE INDEX IF NOT EXISTS idx_documents_updated ON documents(updated_at);
CREATE INDEX IF NOT EXISTS idx_documents_genre ON documents(genre);
CREATE INDEX IF NOT EXISTS idx_document_tags_tag ON document_tags(tag_id);
CREATE INDEX IF NOT EXISTS idx_tags_name ON tags(name);

-- Genre vocabulary (controlled, validated in code)
-- Valid genres: overview, gotchas, architecture, deep-dive, adr, runbook, rfc, guide, reference

-- =============================================================================
-- MEMORY SYSTEM: Content chunks for retrieval during collaboration
-- =============================================================================

-- FTS5 virtual table for full-text search on chunk content
-- tokenize='porter' enables stemming (configure -> configuration)
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    content,                          -- the actual text chunk
    source,                           -- origin: file path or "session:YYYY-MM-DD"
    section,                          -- heading path: "## Config > ### ALB"
    tags,                             -- space-separated tags for FTS matching
    tokenize='porter unicode61'
);

-- Chunk metadata (for queries that don't need FTS)
CREATE TABLE IF NOT EXISTS chunk_meta (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,             -- file path or session identifier
    section TEXT,                     -- heading/section path
    tags TEXT,                        -- comma-separated tags
    chunk_type TEXT DEFAULT 'doc',    -- 'doc', 'session', 'decision', 'note'
    created_at TEXT NOT NULL,
    content_hash TEXT UNIQUE          -- SHA256 hash for deduplication
);

-- Index for chunk queries
CREATE INDEX IF NOT EXISTS idx_chunk_meta_source ON chunk_meta(source);
CREATE INDEX IF NOT EXISTS idx_chunk_meta_type ON chunk_meta(chunk_type);
CREATE INDEX IF NOT EXISTS idx_chunk_meta_created ON chunk_meta(created_at);
