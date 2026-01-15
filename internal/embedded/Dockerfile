FROM node:20-slim

LABEL maintainer="jeanhaley32"
LABEL description="Portable Claude Code development environment"

# Install system dependencies including zsh and sudo
RUN apt-get update && apt-get install -y \
    git \
    curl \
    gh \
    jq \
    ripgrep \
    zsh \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user with sudo access
RUN useradd -m -s /bin/zsh claude && \
    mkdir -p /claude-env /workspace && \
    chown -R claude:claude /claude-env /workspace && \
    echo "claude ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/claude

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

# Install Oh My Zsh
RUN sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended && \
    sed -i 's/ZSH_THEME="robbyrussell"/ZSH_THEME="agnoster"/' ~/.zshrc

# Copy Oh My Zsh to /etc/skel for use with custom HOME directories
USER root
RUN cp -r /home/claude/.oh-my-zsh /etc/skel/.oh-my-zsh && \
    cp /home/claude/.zshrc /etc/skel/.zshrc

# Add init script to set up Oh My Zsh in $HOME if missing
RUN cat > /etc/zsh/zshenv << 'INITSCRIPT'
# Initialize Oh My Zsh in $HOME if not present
if [ -n "$HOME" ] && [ ! -d "$HOME/.oh-my-zsh" ]; then
    mkdir -p "$HOME"
    cp -r /etc/skel/.oh-my-zsh "$HOME/.oh-my-zsh" 2>/dev/null || true
    cp /etc/skel/.zshrc "$HOME/.zshrc" 2>/dev/null || true
fi
INITSCRIPT

USER claude

# Environment variables
ENV CLAUDE_ENV_PATH=/claude-env
ENV ANTHROPIC_API_KEY_FILE=/claude-env/auth/api-key
ENV SHELL=/bin/zsh

# Entry point
ENTRYPOINT ["/bin/zsh"]
