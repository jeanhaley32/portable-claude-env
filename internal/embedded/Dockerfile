FROM node:20-slim

LABEL maintainer="jeanhaley32"
LABEL description="Portable Claude Code development environment"

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    curl \
    gh \
    jq \
    ripgrep \
    && rm -rf /var/lib/apt/lists/*

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user
RUN useradd -m -s /bin/bash claude && \
    mkdir -p /claude-env /workspace && \
    chown -R claude:claude /claude-env /workspace

# Set up volume mount points
VOLUME ["/claude-env", "/workspace"]

# Add workspace symlink setup script
RUN cat > /usr/local/bin/setup-workspace-symlink.sh << 'SCRIPT'
#!/bin/bash
set -e

REPO_ID="$1"
if [ -z "$REPO_ID" ]; then
    echo "Usage: setup-workspace-symlink.sh <repo-id>" >&2
    exit 1
fi

TARGET="/claude-env/repos/${REPO_ID}"
LINK="/workspace/_docs"
TEMP="${LINK}.tmp.$$"

# Ensure target directory exists
mkdir -p "$TARGET"

# Atomic symlink: create temp, then rename
ln -sfn "$TARGET" "$TEMP"
mv -f "$TEMP" "$LINK"

echo "Symlink created: $LINK -> $TARGET"
SCRIPT
RUN chmod +x /usr/local/bin/setup-workspace-symlink.sh

# Switch to non-root user
USER claude
WORKDIR /workspace

# Environment variables
ENV CLAUDE_ENV_PATH=/claude-env
ENV ANTHROPIC_API_KEY_FILE=/claude-env/auth/api-key

# Entry point
ENTRYPOINT ["/bin/bash"]
