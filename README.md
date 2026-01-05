# Portable Claude Code Environment

A containerized, security-focused development environment for Claude Code that can be deployed across different workspaces while keeping credentials and context centralized in an encrypted volume.

## Features

- **Portable**: Single encrypted file contains all credentials and context
- **Secure**: LUKS/DMG encryption keeps credentials safe at rest
- **Cross-platform**: Works on macOS (hdiutil) and Linux (cryptsetup)
- **Persistent context**: Shadow documentation travels with you across machines

## Installation

### Prerequisites

- Go 1.21+
- Docker
- macOS: hdiutil (built-in)
- Linux: cryptsetup (`apt install cryptsetup`)

### Build

```bash
go build -o claude-env ./cmd/claude-env
```

## Usage

### Bootstrap (first time)

```bash
claude-env bootstrap --size 2 --api-key YOUR_API_KEY
```

### Start environment

```bash
cd ~/projects/my-app
claude-env start
```

### Stop environment

```bash
claude-env stop
```

### Check status

```bash
claude-env status
```

## Architecture

See [claude-workflow.md](claude-workflow.md) for detailed design documentation.

## Security

- Encrypted volume only decrypted during active sessions
- Credentials never stored in Docker image
- Shadow documentation (`_docs/`) always gitignored
- Automatic cleanup on exit

## License

Private - All rights reserved
