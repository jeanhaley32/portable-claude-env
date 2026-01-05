# Portable Claude Code Docker Environment

## Project Overview
A containerized, security-focused development environment for Claude Code that can be deployed across different workspaces while keeping credentials and context centralized in an encrypted volume. This is a **personal AI-augmented learning and development environment** that maintains strict separation from version control systems.

**Implementation**: Built in Go with interface-based architecture for cross-platform support (macOS + Linux).

## Goals
- Enable portable Claude Code usage across multiple machines and workspaces
- Secure credential storage using encryption
- Maintain conversation context and history across sessions
- Simple one-command startup from any project directory
- Accelerate learning and understanding of unfamiliar codebases
- Create persistent AI context that travels with you
- Cross-platform support (macOS and Linux)
- Single binary distribution with no runtime dependencies

## Critical Security Boundary
**Nothing from the encrypted volume ever touches git.** This system is intentionally separated from version control:
- Shadow documentation (`_docs/`) is always gitignored
- All AI-generated context stays in the encrypted volume
- Any material destined for the repository must be manually extracted and moved via a separate process
- This is a personal workflow - others can adopt it independently with their own encrypted volumes

## Platform Support
**Supported Operating Systems**:
- **macOS**: Uses native `hdiutil` for encrypted DMG creation
- **Linux**: Uses `cryptsetup` with LUKS encryption
- **Windows**: Not supported (use WSL2 with Linux method)

**Cross-Platform Design**:
- Single Go binary works on both macOS and Linux
- OS detection automatic
- Platform-specific encryption handled via interfaces

## Architecture

### Implementation Language
**Go** - Chosen for:
- Single binary distribution (no dependencies)
- Native cross-compilation (build for macOS/Linux from either platform)
- Excellent standard library for file operations and process execution
- Strong interface support for clean OS abstraction
- Robust error handling
- Easy testing with mockable interfaces

### Core Interfaces

```go
// VolumeManager handles OS-specific encrypted volume operations
type VolumeManager interface {
    Bootstrap(config BootstrapConfig) error
    Mount(volumePath string) (mountPoint string, err error)
    Unmount(mountPoint string) error
    Exists(volumePath string) bool
    GetVolumePath(baseDir string) string
}

// DockerManager handles container operations
type DockerManager interface {
    Start(config ContainerConfig) error
    Stop(containerName string) error
    IsRunning(containerName string) bool
}

// SymlinkManager handles _docs symlink creation and cleanup
type SymlinkManager interface {
    CreateSymlink(workspacePath, targetPath, repoID string) error
    RemoveSymlink(workspacePath string) error
    SymlinkExists(workspacePath string) bool
}

// RepoIdentifier generates consistent repository IDs
type RepoIdentifier interface {
    GetRepoID(workspacePath string) (string, error)
}
```

### OS-Specific Implementations

**MacOS (MacOSVolumeManager)**:
- Uses `hdiutil` for encrypted DMG creation and mounting
- Volume file: `claude-env.dmg`
- Encryption: AES-256
- Mount point: `/Volumes/ClaudeEnv`

**Linux (LinuxVolumeManager)**:
- Uses `cryptsetup` with LUKS encryption
- Volume file: `claude-env.img`
- Encryption: LUKS2 with AES
- Mount point: `/tmp/claude-env-mount`

### Shadow Documentation Strategy

**Symlink Approach** (No sync required):
- Instead of copying files between workspace and encrypted volume, create a symbolic link
- `workspace/_docs/` → `/claude-env/repos/{repo-id}/`
- Changes are instant and automatic (single source of truth)
- No sync conflicts, no bidirectional copying
- Docker follows symlinks inside mounted directories

**How it works**:
1. Encrypted volume mounted to host (e.g., `/Volumes/ClaudeEnv`)
2. Symlink created: `workspace/_docs` points to `/Volumes/ClaudeEnv/repos/{repo-id}/`
3. Both workspace and encrypted volume mounted to container
4. Container accesses `_docs/` through symlink - reads/writes go directly to encrypted volume
5. On exit, symlink removed, volume unmounted

**Benefits**:
- Real-time synchronization (no lag)
- No sync conflicts to resolve
- Simpler implementation (no SyncManager needed)
- Encrypted volume remains single source of truth

### Components

#### 1. Docker Container
- **Image Name**: `portable-claude:latest`
- **Container Name**: `portable-claude` (single shared instance)
- **Pre-installed Tools**:
  - Claude Code CLI
  - GitHub CLI (`gh`)
  - Git, curl, and common dev tools
  - Node.js runtime
- **Mount Points**:
  - `/claude-env` - Read-write mount for encrypted volume content
  - `/workspace` - Read-write mount for project files
- **Updates**: Rebuild image to update Claude Code or tools

#### 2. Encrypted Volume (`claude-env.img`)
- **Technology**: LUKS encryption
- **Mount Point**: `/claude-env` in container
- **Internal Structure**:
  ```
  /claude-env/
  ├── auth/              # API keys, authentication tokens
  ├── config/            # User preferences, Claude Code settings
  ├── claude-context/    # .claude conversation history
  ├── bootstrap/         # Templates and starting files
  └── repos/             # Per-repository documentation and context
      ├── {repo-id-1}/
      └── {repo-id-2}/
  ```
- **Size**: 500MB - 2GB (adjustable based on documentation needs)

#### 3. Quickstart Script (`quickstart.sh`)
**REPLACED BY**: Go CLI binary (`claude-env`)

**CLI Commands**:
```bash
# Bootstrap new environment
claude-env bootstrap [--size 2] [--api-key KEY] [--path .]

# Start existing environment (default command)
claude-env start [--workspace PATH]

# Stop running container
claude-env stop

# Check status
claude-env status

# Show version
claude-env version
```

**Modes**:
- **Bootstrap Mode**: Create new encrypted volume and initialize structure
- **Start Mode**: Mount existing volume and launch container (default)

**Responsibilities**:
- OS detection and appropriate VolumeManager selection
- Encrypted volume management (mount/unmount)
- Docker container lifecycle
- Shadow documentation sync (via SyncManager)
- Cleanup on exit/interrupt

## Workflow

```
User navigates to project subdirectory
    ↓
./quickstart.sh
    ↓
Prompts for encryption password
    ↓
Decrypts and mounts credentials volume
    ↓
Starts Docker container
    ↓
Container mounts:
  - Decrypted credentials → /credentials
  - Parent directory → /workspace
    ↓
Container checks for _docs/ directory in workspace
    ↓
If _docs/ exists:
  - Sync with /credentials/shadow-docs/{repo-name}/
  - Load context documents for Claude
    ↓
Claude Code ready to use with repo-specific context
    ↓
User exits container
    ↓
Sync _docs/ changes back to encrypted volume
    ↓
Auto-cleanup: stop container → unmount → cleanup
```

## File Structure

```
portable-claude-env/
├── cmd/
│   └── claude-env/
│       └── main.go                 # CLI entry point
├── internal/
│   ├── volume/
│   │   ├── interface.go           # VolumeManager interface
│   │   ├── macos.go               # macOS implementation (hdiutil)
│   │   ├── linux.go               # Linux implementation (LUKS)
│   │   └── factory.go             # OS detection & VolumeManager creation
│   ├── docker/
│   │   ├── interface.go           # DockerManager interface
│   │   └── manager.go             # Docker operations
│   ├── sync/
│   │   ├── interface.go           # SyncManager interface
│   │   └── manager.go             # _docs sync operations
│   ├── config/
│   │   └── config.go              # Configuration structs
│   └── platform/
│       └── detect.go              # OS detection utilities
├── Dockerfile                      # Claude Code container definition
├── go.mod                          # Go module definition
├── go.sum                          # Go dependencies
├── README.md                       # User documentation
├── claude-env.dmg                  # (macOS) Encrypted volume
└── claude-env.img                  # (Linux) Encrypted volume
```

## Implementation Steps

### Phase 1: Volume Creation
1. Create encrypted volume file
2. Format with LUKS encryption
3. Create filesystem inside encrypted volume
4. Set up directory structure inside volume

### Phase 2: Docker Image
1. Write Dockerfile with Claude Code installation
2. Configure proper permissions and user setup
3. Set up volume mount points
4. Build and test image

### Phase 3: Quickstart Script
1. Implement decryption logic
2. Add volume mounting
3. Configure Docker run command with proper mounts
4. Implement cleanup trap handlers
5. Add error handling

### Phase 4: Bootstrap Setup
1. Create initial configuration templates
2. Set up credential management
3. Add example context files
4. Document credential setup process

### Phase 5: Shadow Documentation System
1. Implement sync logic for `_docs/` directories
2. Create repository identification mechanism
3. Build bidirectional sync on container start/stop
4. Add conflict resolution for concurrent edits
5. Create templates for common documentation types
6. Implement search/reference capability across shadow docs

## Security Considerations

- Encrypted volume uses LUKS with strong passphrase
- Credentials mounted read-only in container
- Decrypted volume only exists during active session
- Automatic cleanup on exit/interrupt
- No credentials stored in Docker image or host filesystem

## Usage Example

```bash
# One-time setup
cd portable-claude-env/
claude-env bootstrap --size 2 --api-key your-key-here

# Daily usage from any project
cd ~/projects/my-app/src/
claude-env start

# Claude Code now has access to ~/projects/my-app/ as workspace
# And any existing _docs/ context is loaded

# Stop if needed
claude-env stop

# Check status
claude-env status
```

## Requirements

### Development Environment
- Go 1.21 or higher
- Docker installed and running
- Platform-specific encryption tools:
  - **macOS**: hdiutil (built-in)
  - **Linux**: cryptsetup (install via package manager)

### Runtime Requirements
- Docker Desktop (macOS) or Docker Engine (Linux)
- 2-5GB available disk space for encrypted volume
- sudo/admin privileges for volume mounting

### Credentials Needed
- Claude API key from https://console.anthropic.com/

### Go Dependencies
```go
require (
    github.com/spf13/cobra v1.8.0           // CLI framework
    github.com/docker/docker v24.0.7        // Docker SDK
    github.com/docker/go-connections v0.4.0 // Docker connections
)
```

### Key Technical Decisions

**CLI Framework**: Cobra
- Industry standard for Go CLI applications
- Excellent command structure and flag handling
- Auto-generated help and documentation

**Docker Integration**: Docker SDK
- Programmatic container control
- Better error handling than shell commands
- Single shared container model (`portable-claude`)

**Container Image**: Pre-built with tools baked in
- Claude Code CLI pre-installed
- GitHub CLI (`gh`) included
- Common dev tools (git, curl, etc.)
- Rebuild image to update tools (acceptable trade-off)

**Shadow Docs**: Symlink approach (not bidirectional sync)
- Real-time access with zero lag
- No sync conflicts or timing issues
- Simpler implementation
- Docker natively follows symlinks in mounted directories

**Workspace Detection**: Git root, else current directory
- If in a git repo → mount git root (full repo context)
- If not in a git repo → mount current directory
- Simple rule, predictable behavior

**Repository Identification**: Sanitized readable names
- Normalize git remote URL (strip protocol, convert SSH format)
- Result: `github.com-user-my-repo`
- Human-readable in `/claude-env/repos/`
- Same repo-id regardless of clone method (HTTPS vs SSH)

**First-Time Repo Handling**: Create empty directory
- No templates or scaffolding
- User builds structure organically
- KISS principle

**Error Recovery**: Auto-cleanup on start
- Detect and remove broken symlinks
- Stop orphaned containers
- Unmount stale volumes
- Silent and automatic - just make it work

## Benefits

1. **Portability**: Single encrypted file + scripts = entire dev environment
2. **Security**: Credentials never exposed on host filesystem
3. **Consistency**: Same Claude Code setup across all machines
4. **Context Preservation**: Conversation history travels with you
5. **Isolation**: Container keeps dependencies separate from host

## Future Enhancements

- Support for multiple encrypted profiles
- Cloud backup integration for encrypted volume
- Auto-update mechanism for Claude Code CLI
- Template system for common project types
- Automatic generation of shadow documentation from code analysis
- Search/indexing capability across all shadow docs
- Export utility for extracting content destined for git (with review step)
- Repo history tracking (which repos have been worked on)
- Volume size management and cleanup tools

## Shadow Documentation Workflow

### Purpose
This is a **personal learning and development system**, not a documentation generator for repositories. The shadow documentation serves to:
- Accelerate your understanding of unfamiliar codebases
- Provide context for Claude to work more effectively
- Preserve your learning artifacts across machines
- Create a scratchpad for exploring solutions with AI assistance

### What This System IS
- **Personal knowledge base** - Your understanding of codebases
- **AI context hydration** - Helping Claude understand repos faster
- **Learning acceleration** - Notes, explanations, "aha moments"
- **Development scratchpad** - Working through solutions with Claude
- **Portable context** - Your knowledge travels across machines

### What This System IS NOT
- Documentation generator for the repository
- Team collaboration tool
- Source of content for git commits
- Anything that touches version control automatically

### The Boundary
```
┌─────────────────────────────────────┐
│  Git Repository (Team/Public)       │
│  - Source code                      │
│  - Official documentation           │
│  - Team commits                     │
└─────────────────────────────────────┘
           ↕ (Manual extraction only)
┌─────────────────────────────────────┐
│  Encrypted Personal Environment     │
│  - _docs/ shadow documentation      │
│  - Your learning notes              │
│  - Claude context files             │
│  - AI-generated explanations        │
│  - Experimental code with Claude    │
└─────────────────────────────────────┘
```

**If you create something valuable for the team:**
1. Manually review it in `_docs/`
2. Rewrite/adapt as needed for team consumption
3. Move it via a separate process
4. Commit through normal git workflow

This maintains complete control over what enters version control.

### Management
1. **Local Shadow Directory**: `_docs/` in workspace (ALWAYS gitignored)
2. **Encrypted Storage**: Lives permanently in `/claude-env/repos/{repo-id}/`
3. **Access Method**: Symlink from workspace to encrypted volume
4. **Real-time Updates**: Changes write directly to encrypted volume (no sync lag)
5. **Complete Isolation**: Never committed to git - symlink only exists while container runs
6. **Persistence**: Documentation survives across machines via encrypted volume

### Directory Structure Example
```
my-project/
├── src/
├── _docs/                          # Local shadow docs (ALWAYS gitignored)
│   ├── architecture.md             # Your understanding of system architecture
│   ├── modules/                    # Per-module explanations you've learned
│   │   ├── auth.md
│   │   └── database.md
│   ├── learning-notes.md           # Personal learning journal
│   ├── experiments/                # Code experiments with Claude
│   │   └── refactor-ideas.md
│   └── context/                    # AI context files
│       ├── key-patterns.md
│       └── gotchas.md
└── .gitignore                      # MUST contain "_docs/"
```

### Content Examples
- "This module handles JWT auth - the refresh token logic is in lines 45-89"
- "Database connection pool is configured in config.ts, watch out for the timeout setting"
- "Working theory: the bug is related to async race condition in the event handler"
- "Claude's explanation of the dependency injection pattern used here"
- "Notes from exploring the codebase with Claude on 2026-01-05"

### Sync Strategy
- **On Container Start**: Check if `_docs/` exists and sync from encrypted volume
- **On Container Exit**: Sync any changes back to encrypted volume
- **Conflict Resolution**: Timestamp-based, with option to keep both versions
- **Repository Identification**: Uses git remote URL or folder name as key

### Benefits
- **Accelerated Onboarding**: Pre-built context helps Claude understand unfamiliar codebases
- **Knowledge Preservation**: Learning artifacts persist across sessions and machines
- **Privacy**: Documentation stays encrypted and separate from git history
- **Flexible**: Works with any repository without modifying version control
- **Personal**: Your unique understanding and notes, not constrained by team documentation standards
- **Safe Experimentation**: Work through ideas with Claude without polluting git history

### Usage Pattern
```bash
# First time in a new repo
cd ~/work/new-project/
/path/to/quickstart.sh

# Claude helps you understand the code and creates _docs/
# Documentation automatically syncs to encrypted volume on exit

# Later, on different machine
cd ~/work/new-project/
/path/to/quickstart.sh
# Your understanding and context documentation is already there!
```

## Notes

- Volume size should be chosen based on expected context history size
- Consider periodic backups of encrypted volume
- Keep quickstart script in PATH or create alias for convenience
- Can be adapted for other AI coding assistants
