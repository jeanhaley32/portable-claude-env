# Portable Claude

A portable, sandboxed environment for Claude Code. Take your encrypted credentials between machines while keeping the AI isolated—it can only access the project you're working on, not your home directory, SSH keys, or other sensitive files.

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
- **Docker** - to run the containerized environment
- **macOS** - currently only macOS is supported (Linux support planned)

## Quick Start

### 1. Install

```bash
git clone https://github.com/jeanhaley32/portable-claude-env.git
cd portable-claude-env
go build -o claude-env ./cmd/claude-env

# Optional: Install to PATH for system-wide access
sudo mv claude-env /usr/local/bin/
```

The Docker image is embedded in the binary and will be built automatically on first use.

### 2. Bootstrap (First Time Only)

Create your encrypted volume:

```bash
claude-env bootstrap --size 2
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
./claude-env start
```

This will:
1. Prompt for your volume password
2. Mount the encrypted volume
3. Start the Docker container
4. Create `_docs/` symlink for shadow documentation
5. Drop you into a bash shell inside the container

### 4. Use Claude Code

Inside the container, authenticate with Claude:

```bash
claude   # Start Claude Code CLI
```

Your credentials are stored in the encrypted volume and persist across sessions.

### 5. Exit and Re-enter

Type `exit` to leave the container. The container stops but the **volume remains mounted** for fast re-entry:

```bash
exit                    # Leave container
./claude-env start      # Quick re-entry (no password needed)
```

### 6. Lock When Done

When you're finished working for the day, lock your credentials:

```bash
./claude-env lock
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

## Multi-Project Support

Each project gets its own isolated container based on the git repository:

```bash
# Terminal 1
cd ~/projects/frontend
claude-env start    # Creates container: claude-a1b2c3d4

# Terminal 2 (simultaneously)
cd ~/projects/backend
claude-env start    # Creates container: claude-e5f6g7h8
```

Both containers share the same encrypted volume for credentials but run independently.

## Scripting & Automation

For CI/CD or scripted workflows, you can provide the password non-interactively:

```bash
# Via environment variable
export CLAUDE_ENV_PASSWORD="your-password"
claude-env start

# Via stdin (useful for secret managers)
echo "your-password" | claude-env start --password-stdin
vault read -field=password secret/claude | claude-env unlock --password-stdin
```

The `unlock` and `lock` commands output parsable KEY=VALUE format to stdout:

```bash
$ claude-env unlock --password-stdin <<< "$CLAUDE_ENV_PASSWORD"
MOUNT_POINT=/tmp/claude-env-abc123
STATUS=mounted
VOLUME_PATH=/path/to/claude-env.sparseimage

$ claude-env lock
STATUS=locked
VOLUME_PATH=/path/to/claude-env.sparseimage
```

## How It Works

### Encrypted Volume

```
claude-env.sparseimage (encrypted, AES-256)
└── /tmp/claude-env-<id>/ (when mounted)
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

**Important:** After `exit`, the volume remains mounted for fast re-entry. Run `claude-env lock` to unmount and fully secure your credentials.

## File Locations

| File | Description |
|------|-------------|
| `claude-env.sparseimage` | Encrypted volume (keep this safe!) |
| `_docs/` | Symlink to shadow documentation (gitignored) |
| `claude-env` | CLI binary |

## Troubleshooting

### "Volume not found"

Run `bootstrap` first, or specify the volume path:
```bash
./claude-env start --volume /path/to/claude-env.sparseimage
```

### "Docker image not found"

The image builds automatically on first start, but you can manually build:
```bash
claude-env build-image
```

### "Docker is not running"

Start Docker Desktop.

### Container exits immediately

Rebuild the Docker image:
```bash
claude-env build-image --force
```

### "operation not permitted" or "file exists" on start

This can happen if Docker's VirtioFS cache has stale entries. The `lock` command automatically clears the cache, so running it again should fix the issue:
```bash
./claude-env lock
./claude-env start
```

## Development

```bash
# Run tests
go test ./...

# Build
go build -o claude-env ./cmd/claude-env

# Rebuild Docker image (updates embedded Dockerfile too)
cp Dockerfile internal/embedded/Dockerfile
go build -o claude-env ./cmd/claude-env
claude-env build-image --force

```

### Extending Claude Context

Use the `--context` flag during bootstrap to specify markdown files that extend Claude's base context:

```bash
# Single context file
claude-env bootstrap --context ./project-rules.md

# Multiple context files
claude-env bootstrap --context ./coding-standards.md --context ./api-guidelines.md

# Or comma-separated
claude-env bootstrap --context ./rules.md,./guidelines.md
```

The context is generated **once at bootstrap** and becomes part of the encrypted volume. After bootstrap, you or the agent can manually edit `~/.claude/CLAUDE.md` inside the container to modify the context.

## License

MIT License

## Contributing

Contributions welcome! Please open an issue or PR.
