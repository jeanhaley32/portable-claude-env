# Portable Claude Code Environment

A containerized, security-focused development environment for Claude Code with encrypted credential storage. Keep your API keys, conversation history, and preferences secure in an encrypted volume that travels with you.

## Why?

When using Claude Code across multiple projects and machines, you face challenges:
- **Credential management**: API keys scattered across machines
- **Context loss**: Conversation history and preferences lost between sessions
- **Security concerns**: Credentials stored in plaintext in home directories

This project solves these by:
- Storing all sensitive data in an **encrypted volume** (AES-256)
- Running Claude Code in a **Docker container** with controlled access
- Persisting your **home directory** in the encrypted volume (credentials survive restarts)
- Creating **shadow documentation** (`_docs/`) for per-project notes that don't pollute your git repo

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

### 5. Exit

Simply type `exit` to leave the container. The environment automatically:
- Stops the container
- Unmounts the encrypted volume (locking your credentials)

You can also manually stop with `claude-env stop` if needed.

## Commands

| Command | Description |
|---------|-------------|
| `bootstrap` | Create new encrypted volume |
| `start` | Mount volume, start container, enter shell |
| `stop` | Stop container, unmount volume (manual) |
| `status` | Show current environment status |
| `build-image` | Build Docker image (automatic on first start) |
| `version` | Show version information |

## How It Works

### Encrypted Volume

```
claude-env.sparseimage (encrypted, AES-256)
└── ~/.claude-env/mount (when mounted)
    ├── auth/           # API keys
    ├── config/         # Settings
    ├── home/           # User home directory (Claude credentials live here)
    ├── repos/          # Per-project shadow documentation
    └── ...
```

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

| Aspect | Protection |
|--------|------------|
| Credentials at rest | AES-256 encrypted volume |
| Credentials in memory | Only decrypted while container runs |
| API keys | Never stored in Docker image or git |
| Volume password | Never stored, prompted each session |
| Container isolation | Docker provides process isolation |

### What's Protected

- Claude API credentials (`~/.claude/`)
- Shell history (`~/.bash_history`)
- Any config files in home directory
- Shadow documentation

### What's NOT Protected

- Your project source code (mounted read-write)
- Network traffic (not encrypted by this tool)
- Runtime memory (standard Docker security applies)

### AI Model Isolation

A key security benefit of this project is **sandboxing the AI model** from your system. When Claude Code runs inside the container, it can only access:

```
/workspace/     ← Your current project (explicitly mounted)
/claude-env/    ← Encrypted volume (home directory, credentials, docs)
```

**The AI model CANNOT access:**
- Your host home directory (`~/`)
- Other projects or repositories
- System files and configurations
- SSH keys, GPG keys, or other credentials outside the encrypted volume
- Parent directories of your project
- Any path not explicitly mounted

This is a significant security improvement over running AI coding assistants directly on your host machine, where they typically have access to your entire home directory and potentially sensitive files.

**Isolation layers:**

| Layer | Protection |
|-------|------------|
| Docker container | Process isolation from host |
| Explicit mounts only | Only two paths visible to AI |
| Non-root user | Container runs as unprivileged `claude` user |
| No host networking | Container uses isolated network namespace |

This design ensures that even if the AI attempts to access files outside its sandbox, the container boundary prevents it.

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

## License

MIT License

## Contributing

Contributions welcome! Please open an issue or PR.
