package embedded

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeanhaley32/claude-capsule/internal/constants"
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
// This is appended to CLAUDE.md during bootstrap.
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

// BeadsProtocolDocs contains the documentation for the beads issue tracker.
// This is appended to CLAUDE.md during bootstrap.
const BeadsProtocolDocs = `
---

## Issue Tracking (Beads)

The **Beads** issue tracker (` + "`bd`" + `) provides local-first, per-project issue tracking.
All data lives on the encrypted volume — nothing is committed to git.

### Commands

` + "```bash" + `
# List all open issues
bd list

# Create a new issue
bd create "Short description of the issue"

# Show details of a specific issue
bd show <issue-id>

# Close an issue
bd close <issue-id>

# Search issues by keyword
bd search "keyword"
` + "```" + `

### Storage

- Data location: ` + "`/claude-env/repos/<project>/.beads/`" + `
- Per-project isolated — switching repos uses a separate database
- Local-only on the encrypted volume — never touches git or GitHub
- Persists across sessions, secured when the volume is locked

### When to Use

- Track bugs, tasks, and TODOs that span multiple sessions
- Record issues discovered during code review or exploration
- Maintain a backlog of work items for the current project
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

// VersionFile is the path within the encrypted volume for version tracking.
const VersionFile = "home/.claude/VERSION"

// WriteDocSyncFiles writes the doc-sync skill files to the mounted volume.
// Files are written with executable permissions for Python scripts.
func WriteDocSyncFiles(mountPoint string) error {
	skillDir := filepath.Join(mountPoint, DocSyncSkillDir)

	// Create the skill directory
	if err := os.MkdirAll(skillDir, constants.DirPermissions); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// File definitions: path, content, permissions
	files := []struct {
		name    string
		content []byte
		perm    os.FileMode
	}{
		{"doctool.py", DoctoolPy, constants.ExecutablePermissions},
		{"mcp_server.py", MCPServerPy, constants.ExecutablePermissions},
		{"schema.sql", SchemaSql, constants.PublicFilePermissions},
		{"SKILL.md", SkillMd, constants.PublicFilePermissions},
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
// If settings.json already exists, merges our mcpServers config into it.
func WriteSettingsJSON(mountPoint string) error {
	claudeDir := filepath.Join(mountPoint, "home", ".claude")

	// Ensure directory exists (should already from VolumeStructure)
	if err := os.MkdirAll(claudeDir, constants.DirPermissions); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Start with our default config
	var settings map[string]interface{}
	if err := json.Unmarshal([]byte(SettingsJSON), &settings); err != nil {
		return fmt.Errorf("failed to parse default settings: %w", err)
	}

	// If existing settings.json exists, merge into it
	if existingData, err := os.ReadFile(settingsPath); err == nil {
		var existing map[string]interface{}
		if err := json.Unmarshal(existingData, &existing); err != nil {
			return fmt.Errorf("failed to parse existing settings.json: %w", err)
		}

		// Merge: preserve existing, add our mcpServers.doc-sync
		settings = mergeSettings(existing, settings)
	}

	// Write merged settings with indentation for readability
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, output, constants.PublicFilePermissions); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	return nil
}

// mergeSettings deep merges src into dst, with src taking precedence for conflicts.
// Specifically handles mcpServers as a nested map to preserve existing servers.
func mergeSettings(dst, src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all existing keys
	for k, v := range dst {
		result[k] = v
	}

	// Merge in source keys
	for k, v := range src {
		if k == "mcpServers" {
			// Special handling: merge mcpServers maps
			result[k] = mergeMCPServers(
				toStringMap(dst[k]),
				toStringMap(v),
			)
		} else {
			// For other keys, src overwrites dst
			result[k] = v
		}
	}

	return result
}

// mergeMCPServers merges MCP server configurations, with src taking precedence.
func mergeMCPServers(dst, src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy existing servers
	for k, v := range dst {
		result[k] = v
	}

	// Add/overwrite with new servers
	for k, v := range src {
		result[k] = v
	}

	return result
}

// toStringMap safely converts interface{} to map[string]interface{}.
func toStringMap(v interface{}) map[string]interface{} {
	if v == nil {
		return make(map[string]interface{})
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return make(map[string]interface{})
}

// WriteVersionFile writes the capsule version to track installed components.
func WriteVersionFile(mountPoint, version string) error {
	versionPath := filepath.Join(mountPoint, VersionFile)

	content := fmt.Sprintf("capsule %s\n", version)
	if err := os.WriteFile(versionPath, []byte(content), constants.PublicFilePermissions); err != nil {
		return fmt.Errorf("failed to write VERSION: %w", err)
	}

	return nil
}
