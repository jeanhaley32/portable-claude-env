package embedded

// ClaudeMDTemplate is the default CLAUDE.md content for new environments.
// This file is written to ~/.claude/CLAUDE.md inside the encrypted volume
// during bootstrap to provide context about the sandboxed environment.
const ClaudeMDTemplate = `# Sandboxed Claude Code Environment

You are running inside a secure, containerized environment. Your access is intentionally restricted to protect the user's system.

## Environment Boundaries

**You CAN access:**
- /workspace/ - The current project (mounted from the host)
- /claude-env/ - Your encrypted home directory and data

**You CANNOT access:**
- The host's home directory (~/)
- Other projects or repositories
- System files, SSH keys, AWS credentials, or other sensitive data
- Any path outside the two directories above

This isolation is a security feature, not a limitation to work around.

## Directory Structure

` + "```" + `
/workspace/              <- Current project (your working directory)
/workspace/_docs/        <- Shadow documentation (persisted in encrypted volume)

/claude-env/             <- Encrypted volume (your $HOME)
├── home/                <- Your home directory ($HOME=/claude-env/home)
│   └── .claude/         <- Claude Code config and credentials
├── repos/               <- Per-project storage
│   └── <project>/
│       └── .beads/      <- Issue tracker (per project)
├── auth/                <- API keys (if stored separately)
└── config/              <- Additional configuration
` + "```" + `

## Shadow Documentation (/workspace/_docs/)

The _docs/ directory is your persistent workspace for this repository. Use it to build and maintain context that helps you understand and work with the codebase effectively.

**What to store in _docs/:**

1. **Architecture Documentation**
   - System design notes and diagrams (as markdown/ASCII)
   - Component relationships and data flow
   - API contracts and interfaces

2. **Codebase Context**
   - Key file summaries and their purposes
   - Important patterns and conventions used
   - Technical debt notes and refactoring plans

3. **Session State**
   - Current task progress and next steps
   - Decisions made and their rationale
   - Open questions to revisit

4. **Reference Material**
   - Useful code snippets and examples
   - Common commands and workflows
   - Environment-specific configuration notes

**Example _docs/ structure:**
` + "```" + `
_docs/
├── architecture.md      <- System design and component overview
├── conventions.md       <- Code style and patterns used
├── current-task.md      <- What you're working on now
├── decisions.md         <- Technical decisions and rationale
├── api-notes.md         <- API documentation and examples
└── troubleshooting.md   <- Known issues and solutions
` + "```" + `

**Why use _docs/:**
- Persists across sessions (stored in encrypted volume)
- Not committed to git (keeps repo clean)
- Helps you resume context quickly after breaks
- Builds institutional knowledge about the codebase

**Best practice:** At the end of a session, update _docs/current-task.md with your progress and next steps. At the start of a session, read it to resume context.

## Working with Projects

1. **Your working directory is /workspace/** - This is the project the user mounted
2. **Build context in _docs/** - Create documentation that helps you understand and navigate the codebase
3. **Credentials persist** - Your Claude authentication is stored in the encrypted volume and survives container restarts

## Session Lifecycle

- **start**: Container starts, encrypted volume is mounted
- **exit**: Container stops, volume stays mounted for quick re-entry
- **lock**: Volume is unmounted, credentials are secured

The user can quickly re-enter with ` + "`capsule start`" + ` without re-entering their password (until they run ` + "`lock`" + `).
`
