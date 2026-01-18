# Claude Capsule

A portable, sandboxed workspace for Claude Code. Take your encrypted credentials between machines while keeping the AI isolated—it can only access the project you're working on, not your home directory, SSH keys, or other sensitive files.

## Why Sandbox Your AI?

When you run Claude Code directly on your machine, it has access to:

```
~/.ssh/           ← Your SSH keys (GitHub, servers, etc.)
~/.aws/           ← AWS credentials
~/.config/        ← Application secrets and tokens
~/.gnupg/         ← GPG keys
~/Projects/       ← ALL your other projects and repos
~/.bash_history   ← Command history with passwords/secrets
~/.netrc          ← Plaintext credentials
...and everything else in your home directory
```

**This tool solves that** by running Claude Code inside a Docker container with strict mount boundaries:

```
┌─────────────────────────────────────────────────────┐
│  What Claude Code can see (sandboxed)               │
├─────────────────────────────────────────────────────┤
│  /workspace/     ← Only YOUR CURRENT project        │
│  /claude-env/    ← Its own encrypted home directory │
│                                                     │
│  That's it. Nothing else.                           │
└─────────────────────────────────────────────────────┘
```

**Additional benefits:**
- **Encrypted credentials** - API keys and session data stored in AES-256 encrypted volume
- **Portable** - Take your encrypted volume between macOS machines
- **Per-project isolation** - Switch projects without credential leakage

## Prerequisites

- **Go 1.21+** - to build the CLI
- **Docker Desktop** - to run the containerized environment
- **macOS** - this tool is macOS-only (uses encrypted sparse images via `hdiutil`)

## Quick Start

### 1. Install

```bash
git clone https://github.com/jeanhaley32/claude-capsule.git
cd claude-capsule
make install
```

This builds the binary and installs it to `~/.local/bin/capsule`. Make sure `~/.local/bin` is in your PATH.

The Docker image is embedded in the binary and will be built automatically on first use.

### 2. Bootstrap (First Time Only)

Create your encrypted volume:

```bash
capsule bootstrap --size 2
```

You'll be prompted to create a password. This creates a 2GB encrypted sparse image.

**Options:**
- `--size N` - Volume size in GB (default: 2)
- `--path PATH` - Where to store the volume (default: current directory)
- `--api-key KEY` - Optionally store your API key during setup

### 3. Start the Environment

Navigate to any project and start:

```bash
cd ~/projects/my-app
capsule start
```

This will:
1. Prompt for your volume password
2. Mount the encrypted volume
3. Start the Docker container
4. Create `_docs/` symlink for shadow documentation
5. Drop you into a fish shell inside the container

### 4. Use Claude Code

Inside the container, authenticate with Claude:

```bash
claude   # Start Claude Code CLI
```

Your credentials are stored in the encrypted volume and persist across sessions.

### 5. Exit and Re-enter

Type `exit` to leave the container. The container stops but the **volume remains mounted** for fast re-entry:

```bash
exit              # Leave container
capsule start     # Quick re-entry (no password needed)
```

### 6. Lock When Done

When you're finished working for the day, lock your credentials:

```bash
capsule lock
```

This unmounts the encrypted volume, securing your credentials. The next `start` will require your password again.


## Commands

| Command | Description |
|---------|-------------|
| `bootstrap` | Create new encrypted volume |
| `start` | Mount volume (if needed), start container, enter shell |
| `stop` | Stop container (keeps volume mounted) |
| `unlock` | Mount encrypted volume without starting container |
| `lock` | Unmount volume and secure credentials |
| `status` | Show current environment status |
| `build-image` | Build Docker image (automatic on first start) |
| `version` | Show version information |

**Start options:**
- `--volume PATH` - Path to encrypted volume (auto-detected if not specified)
- `--workspace PATH` - Workspace path (defaults to git root or current directory)

## Multi-Project Support

Each project gets its own isolated container based on the git repository:

```bash
# Terminal 1
cd ~/projects/frontend
capsule start    # Creates container: claude-a1b2c3d4

# Terminal 2 (simultaneously)
cd ~/projects/backend
capsule start    # Creates container: claude-e5f6g7h8
```

Both containers share the same encrypted volume for credentials but run independently.

## Scripting & Automation

For CI/CD or scripted workflows, use `unlock` to mount the volume non-interactively:

```bash
# Via environment variable
export CAPSULE_PASSWORD="your-password"
capsule unlock

# Via stdin (useful for secret managers)
echo "your-password" | capsule unlock --password-stdin
vault read -field=password secret/claude | capsule unlock --password-stdin
```

Once unlocked, `capsule start` will use the already-mounted volume without prompting for a password.

The `unlock` and `lock` commands output parsable KEY=VALUE format to stdout:

```bash
$ capsule unlock --password-stdin <<< "$CAPSULE_PASSWORD"
MOUNT_POINT=/tmp/capsule-abc123
STATUS=mounted
VOLUME_PATH=/path/to/capsule.sparseimage

$ capsule lock
STATUS=locked
VOLUME_PATH=/path/to/capsule.sparseimage
```

## How It Works

### Encrypted Volume

```
capsule.sparseimage (encrypted, AES-256)
└── /tmp/capsule-<id>/ (when mounted)
    ├── auth/           # API keys
    ├── config/         # Settings
    ├── home/           # User home directory (Claude credentials live here)
    ├── repos/          # Per-project shadow documentation
    └── ...
```

The volume mounts to a unique path in `/tmp` for Docker compatibility.

### Container Architecture

```
┌─────────────────────────────────────────┐
│ Docker Container                        │
│                                         │
│  /workspace  ← Your project (mounted)   │
│  /workspace/_docs → /claude-env/repos/  │
│                                         │
│  /claude-env ← Encrypted volume         │
│  $HOME = /claude-env/home               │
│                                         │
│  Claude Code CLI installed              │
└─────────────────────────────────────────┘
```

### Container Environment

The container comes pre-configured with:

| Tool | Description |
|------|-------------|
| **fish shell** | Modern shell with syntax highlighting and autosuggestions |
| **Starship** | Cross-shell prompt with gruvbox-rainbow theme |
| **Claude Code** | Anthropic's AI coding assistant CLI |
| **gh** | GitHub CLI for PRs, issues, and repo management |
| **git** | Version control |
| **ripgrep** | Fast recursive search |
| **jq** | JSON processor |
| **sudo** | The `claude` user has passwordless sudo access |

**Updating Claude Code**: Run `claude-upgrade` inside the container to update to the latest version.

### Shadow Documentation (`_docs/`)

Each project gets a `_docs/` symlink that points to project-specific storage in the encrypted volume:

```
~/projects/my-app/_docs → /claude-env/repos/github.com-user-my-app/
```

Use this for:
- Notes and context for Claude
- Drafts and work-in-progress
- Anything you don't want in git

The symlink is created inside the container and is automatically gitignored.

## Security Model

### Isolation Layers

| Layer | Protection |
|-------|------------|
| Docker container | Process isolation from host |
| Explicit mounts only | Only `/workspace` and `/claude-env` visible |
| Non-root user | Container runs as unprivileged `claude` user |
| No host networking | Isolated network namespace |
| Encrypted volume | AES-256 encryption for credentials at rest |

### What's Protected

- Your host system (SSH keys, other projects, system files)
- Claude API credentials (encrypted at rest)
- Shell history and config files (inside encrypted volume)

### What's NOT Protected

- Your current project source code (mounted read-write by design)
- Network traffic (not encrypted by this tool)
- Runtime memory (standard Docker security applies)

**Important:** After `exit`, the volume remains mounted for fast re-entry. Run `capsule lock` to unmount and fully secure your credentials.

## File Locations

| File | Description |
|------|-------------|
| `capsule.sparseimage` | Encrypted volume (keep this safe!) |
| `_docs/` | Symlink to shadow documentation (gitignored) |
| `~/.local/bin/capsule` | CLI binary (after `make install`) |

## Troubleshooting

### "Volume not found"

Run `bootstrap` first, or specify the volume path:
```bash
capsule start --volume /path/to/capsule.sparseimage
```

### "Docker image not found"

The image builds automatically on first start, but you can manually build:
```bash
capsule build-image
```

### "Docker is not running"

Start Docker Desktop.

### Container exits immediately

Rebuild the Docker image:
```bash
capsule build-image --force
```

### "operation not permitted" or "file exists" on start

This can happen if Docker's VirtioFS cache has stale entries. The `lock` command automatically clears the cache, so running it again should fix the issue:
```bash
capsule lock
capsule start
```

## Development

```bash
make build      # Build the binary
make install    # Build and install to ~/.local/bin
make test       # Run tests
make docker     # Sync Dockerfile and rebuild Docker image
make clean      # Remove build artifacts
make uninstall  # Remove from ~/.local/bin
```

### Extending Claude Context

Use the `--context` flag during bootstrap to specify markdown files that extend Claude's base context:

```bash
# Single context file
capsule bootstrap --context ./project-rules.md

# Multiple context files
capsule bootstrap --context ./coding-standards.md --context ./api-guidelines.md

# Or comma-separated
capsule bootstrap --context ./rules.md,./guidelines.md
```

The context is generated **once at bootstrap** and becomes part of the encrypted volume. After bootstrap, you or the agent can manually edit `~/.claude/CLAUDE.md` inside the container to modify the context.

## License

MIT License

## Contributing

Contributions welcome! Please open an issue or PR at [github.com/jeanhaley32/claude-capsule](https://github.com/jeanhaley32/claude-capsule).
