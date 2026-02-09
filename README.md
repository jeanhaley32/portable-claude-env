# Claude Capsule

A secure, portable workspace for AI-assisted development—isolating your code, your credentials, and your context.

## What It Does

Capsule runs Claude Code inside a Docker container. The container can only see two things: your current project and its own encrypted home directory.

Your host machine—SSH keys, cloud credentials, other projects—is invisible. Credentials you set up inside the container persist in the encrypted volume and travel with it.

## Workspace Organization

Working with an LLM generates artifacts: planning docs, architecture notes, task logs. The encrypted volume gives these a home outside your git history. Each project gets a `_docs/` directory that persists across sessions.

Delete the volume, delete everything. Move it to another machine, your context comes with you.

```
┌─────────────────────────────────────────────────────┐
│ Your Machine                                        │
│                                                     │
│  ~/.ssh, ~/.aws, ~/.config    ← Not visible        │
│  ~/other-projects/            ← Not visible        │
│                                                     │
│  ~/.capsule/volumes/capsule.sparseimage            │
└─────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────┐
│ Container                                           │
│                                                     │
│  /workspace/        ← Your project                 │
│  /workspace/_docs/  → symlink to repos/<project>/  │
│                                                     │
│  /claude-env/       ← Encrypted volume mount       │
│  ├── home/          ← $HOME                        │
│  │   └── .claude/   ← Claude Code credentials      │
│  │       └── skills/← doc-sync + memory system      │
│  ├── repos/         ← Shadow docs + memory DB      │
│  └── auth/          ← API keys                     │
│                                                     │
└─────────────────────────────────────────────────────┘
```

## Prerequisites

- **macOS** — uses encrypted sparse images via `hdiutil`
- **Docker Desktop** — runs the containerized environment
- **Go 1.21+** — builds the CLI

## Quick Start

### 1. Install

```bash
git clone https://github.com/jeanhaley32/claude-capsule.git
cd claude-capsule
make install
```

Installs to `~/.local/bin/capsule`. Ensure `~/.local/bin` is in your PATH.

### 2. Bootstrap

Create your encrypted workspace:

```bash
capsule bootstrap
```

You'll be prompted for:
- **Location** — Global (`~/.capsule/volumes/`) or Local (`./capsule.sparseimage`)
- **Size** — Volume size in GB (default: 2)
- **Password** — Encryption password

**Flags to skip prompts:**
- `--global` — Use global location (recommended)
- `--local` — Use current directory
- `--volume PATH` — Explicit path
- `--size N` — Volume size in GB
- `--api-key KEY` — Store API key during setup

### 3. Start

Navigate to any project and start:

```bash
cd ~/projects/my-app
capsule start
```

This will:
1. Prompt for your password (if volume not mounted)
2. Mount the encrypted volume
3. Start the container
4. Create `_docs/` symlink for shadow documentation
5. Drop you into a fish shell

### 4. Work

Inside the container:

```bash
claude   # Start Claude Code
```

Your credentials persist in the encrypted volume across sessions.

### 5. Exit and re-enter

```bash
exit              # Leave container
capsule start     # Quick re-entry (no password needed—volume still mounted)
```

### 6. Lock when done

```bash
capsule lock
```

Unmounts the encrypted volume, securing your credentials. Next `start` requires your password.

## Commands

| Command | Description |
|---------|-------------|
| `bootstrap` | Create encrypted workspace |
| `start` | Mount, start container, enter shell |
| `stop` | Stop container (keeps volume mounted) |
| `unlock` | Mount volume without starting container |
| `lock` | Unmount volume and secure credentials |
| `status` | Show environment status |
| `build-image` | Build Docker image |
| `version` | Show version |

**Common flags:**
- `--volume PATH` — Path to encrypted volume (auto-detected if not specified)
- `--workspace PATH` — Workspace path (defaults to git root or current directory)

## Volume Location

Capsule checks for volumes in this order:

1. **Explicit path** — `--volume /path/to/volume.sparseimage`
2. **Local volume** — `./capsule.sparseimage` (if exists)
3. **Global volume** — `~/.capsule/volumes/capsule.sparseimage` (default)

Global storage (recommended) lets you access the same credentials from any project directory.

## Multi-Project Support

Each project gets its own container based on the git repository:

```bash
# Terminal 1
cd ~/projects/frontend
capsule start    # Container: claude-a1b2c3d4

# Terminal 2
cd ~/projects/backend
capsule start    # Container: claude-e5f6g7h8
```

Both containers share the encrypted volume but run independently.

## Shadow Documentation

Each project gets a `_docs/` symlink pointing to persistent storage in the encrypted volume:

```
~/projects/my-app/_docs → /claude-env/repos/github.com-user-my-app/
```

Use this for:
- Planning documents and architecture notes
- Progress logs and decision records
- Context that shouldn't live in git

The symlink is created inside the container. Add `_docs` to your `.gitignore` to keep it out of version control.

## Memory System

Every bootstrapped volume includes the doc-sync skill—a SQLite-backed memory system that persists decisions, context, and learnings across sessions.

This includes:
- **doc-sync skill** — Documentation lifecycle tooling and MCP server
- **Memory database** — FTS5-powered search over stored memories
- **MCP integration** — Claude Code can search and add memories natively

Inside the container, Claude automatically queries memory at session start and stores conclusions before context rolls. The database lives in the encrypted volume alongside your project's shadow docs.

Memory commands (inside container):
```bash
# Search past decisions
python3 ~/.claude/skills/doc-sync/doctool.py memory search "auth strategy"

# Store a decision
python3 ~/.claude/skills/doc-sync/doctool.py memory add "Chose JWT over sessions" \
    --tags auth,api --type decision

# View recent memories
python3 ~/.claude/skills/doc-sync/doctool.py memory recent
```

## Scripting & Automation

For CI/CD or scripted workflows:

```bash
# Via environment variable
export CAPSULE_PASSWORD="your-password"
capsule unlock

# Via stdin
echo "your-password" | capsule unlock --password-stdin
vault read -field=password secret/claude | capsule unlock --password-stdin
```

Output is parsable KEY=VALUE format:

```bash
$ capsule unlock
MOUNT_POINT=/Volumes/Capsule-abc123
STATUS=mounted
VOLUME_PATH=/Users/you/.capsule/volumes/capsule.sparseimage
```

## Container Environment

Pre-configured tools:

| Tool | Description |
|------|-------------|
| **fish** | Modern shell with syntax highlighting |
| **Starship** | Cross-shell prompt (gruvbox-rainbow theme) |
| **Claude Code** | Anthropic's AI coding assistant |
| **Beads (bd)** | Local-first issue tracker (per-project, on encrypted volume) |
| **gh** | GitHub CLI |
| **git** | Version control |
| **ripgrep** | Fast recursive search |
| **jq** | JSON processor |
| **Python 3** | Required for doc-sync memory system |
| **sudo** | Passwordless sudo for `claude` user |

Update Claude Code: `claude-upgrade`

## Security Model

| Layer | Protection |
|-------|------------|
| Docker container | Process isolation from host |
| Explicit mounts | Only `/workspace` and `/claude-env` visible |
| Non-root user | Runs as unprivileged `claude` user |
| No host networking | Isolated network namespace |
| Encrypted volume | AES-256 encryption at rest |

**Protected:** Host system, SSH keys, other projects, credentials at rest

**Not protected:** Current project (mounted read-write by design), network traffic, runtime memory

**Important:** After `exit`, the volume remains mounted for fast re-entry. Run `capsule lock` to fully secure credentials.

## Troubleshooting

### "Volume not found"

Capsule checks local then global locations. Either:
- Run `capsule bootstrap` to create a volume
- Specify path: `capsule start --volume /path/to/capsule.sparseimage`

### "Docker image not found"

Builds automatically on first start. Manual build:
```bash
capsule build-image
```

### "Docker is not running"

Start Docker Desktop.

### Container exits immediately

Rebuild the image:
```bash
capsule build-image --force
```

### "operation not permitted" or "file exists"

Docker's VirtioFS cache has stale entries. Lock and restart:
```bash
capsule lock
capsule start
```

## Development

```bash
make build      # Build binary
make install    # Install to ~/.local/bin
make test       # Run tests
make docker     # Rebuild Docker image
make clean      # Remove build artifacts
make uninstall  # Remove from ~/.local/bin
```

### Extending Claude Context

Add markdown files to Claude's base context during bootstrap:

```bash
capsule bootstrap --context ./coding-standards.md --context ./api-guidelines.md
```

Context is generated once at bootstrap. Edit `~/.claude/CLAUDE.md` inside the container to modify later.

## License

MIT License

## Contributing

Contributions welcome at [github.com/jeanhaley32/claude-capsule](https://github.com/jeanhaley32/claude-capsule).
