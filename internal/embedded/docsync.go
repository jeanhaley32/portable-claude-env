package embedded

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed docsync/doctool.py
var DoctoolPy []byte

//go:embed docsync/mcp_server.py
var MCPServerPy []byte

//go:embed docsync/schema.sql
var SchemaSql []byte

//go:embed docsync/SKILL.md
var SkillMd []byte

// MemoryProtocolDocs contains the documentation for the memory retrieval system.
// This is appended to CLAUDE.md when --with-memory is used.
const MemoryProtocolDocs = `
---

## Memory Retrieval Protocol

The **Collaboration Memory** system provides long-term memory for AI collaboration.
Use it to recall past decisions, context, and learnings across sessions.

### When to Query

**At session start**, after understanding the user's first request:
1. Query memory with task-relevant keywords: ` + "`memory_search \"topic keywords\"`" + `
2. Load relevant results into working context
3. Reference prior decisions when they apply to current work

**During collaboration**, query memory when:
- Encountering an unfamiliar component
- Making architectural or design decisions
- About to ask the user something that may have been discussed before

**Before context rolls or session ends**:
1. Identify decisions, gotchas, or solutions worth preserving
2. Store via ` + "`memory_add`" + ` with appropriate tags and type
3. Prefer conclusions over discussion transcripts

### Staleness Detection

Memory results include age information to help assess relevance:

| Age | Classification | Interpretation |
|-----|----------------|----------------|
| 0-7 days | Fresh | Recent context, likely current |
| 8-30 days | Recent | Probably still valid |
| 31-90 days | Aging | Verify before relying on |
| 90+ days | **STALE** | Validate with user or codebase |

**Output format:**
` + "```" + `
─── Result 1 [STALE: 95 days] ───
Source: session:2025-10-23 (95 days ago)
Section: Note
Tags: ecs,routing
Type: decision

Decision content here...
` + "```" + `

**When you encounter stale results:**
1. **Don't ignore them** — stale doesn't mean wrong, just unverified
2. **Cross-reference** — check if the codebase still matches the decision
3. **Ask if uncertain** — "I found a decision from 3 months ago about X. Is this still current?"
4. **Update if outdated** — add a new memory with current information

### Commands

` + "```bash" + `
# Search memory (results include age)
python3 ~/.claude/skills/doc-sync/doctool.py memory search "ECS routing"

# Add a decision
python3 ~/.claude/skills/doc-sync/doctool.py memory add "Decision text" \
    --tags topic,area --type decision

# Check recent additions (shows age)
python3 ~/.claude/skills/doc-sync/doctool.py memory recent

# Look back further
python3 ~/.claude/skills/doc-sync/doctool.py memory recent --days 30
` + "```" + `

**MCP tools**: ` + "`memory_search`" + `, ` + "`memory_add`" + `, ` + "`memory_recent`" + `

All return ` + "`age_days`" + ` and ` + "`is_stale`" + ` fields for programmatic staleness handling.

This is not optional. Query before asking. Write before forgetting.
`

// SettingsJSON is the Claude Code settings.json that configures the MCP server.
const SettingsJSON = `{
  "mcpServers": {
    "doc-sync": {
      "command": "python3",
      "args": ["/claude-env/home/.claude/skills/doc-sync/mcp_server.py"]
    }
  }
}
`

// DocSyncSkillDir is the path within the encrypted volume for doc-sync files.
const DocSyncSkillDir = "home/.claude/skills/doc-sync"

// WriteDocSyncFiles writes the doc-sync skill files to the mounted volume.
// Files are written with executable permissions for Python scripts.
func WriteDocSyncFiles(mountPoint string) error {
	skillDir := filepath.Join(mountPoint, DocSyncSkillDir)

	// Create the skill directory
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// File definitions: path, content, permissions
	files := []struct {
		name    string
		content []byte
		perm    os.FileMode
	}{
		{"doctool.py", DoctoolPy, 0755},
		{"mcp_server.py", MCPServerPy, 0755},
		{"schema.sql", SchemaSql, 0644},
		{"SKILL.md", SkillMd, 0644},
	}

	for _, f := range files {
		path := filepath.Join(skillDir, f.name)
		if err := os.WriteFile(path, f.content, f.perm); err != nil {
			return fmt.Errorf("failed to write %s: %w", f.name, err)
		}
	}

	return nil
}

// WriteSettingsJSON writes the Claude Code settings.json to configure MCP servers.
// This overwrites any existing settings.json (merge support deferred to future phase).
func WriteSettingsJSON(mountPoint string) error {
	claudeDir := filepath.Join(mountPoint, "home", ".claude")

	// Ensure directory exists (should already from VolumeStructure)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(SettingsJSON), 0644); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	return nil
}
