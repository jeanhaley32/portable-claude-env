FROM node:20-slim

LABEL maintainer="jeanhaley32"
LABEL description="Claude Capsule workspace environment"

# Install system dependencies including fish, fonts, and Python
RUN apt-get update && apt-get install -y \
    git \
    curl \
    gh \
    jq \
    ripgrep \
    fish \
    sudo \
    fontconfig \
    unzip \
    python3 \
    && rm -rf /var/lib/apt/lists/*

# Install Nerd Font (FiraCode)
RUN mkdir -p /usr/local/share/fonts && \
    curl -fLo /tmp/FiraCode.zip https://github.com/ryanoasis/nerd-fonts/releases/latest/download/FiraCode.zip && \
    unzip /tmp/FiraCode.zip -d /usr/local/share/fonts/FiraCode && \
    rm /tmp/FiraCode.zip && \
    fc-cache -fv

# Install Starship prompt
RUN curl -sS https://starship.rs/install.sh | sh -s -- -y

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Install Beads issue tracker
RUN npm install -g @beads/bd

# Create non-root user with sudo access
RUN useradd -m -s /usr/bin/fish claude && \
    mkdir -p /claude-env /workspace && \
    chown -R claude:claude /claude-env /workspace && \
    echo "claude ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/claude

# Global gitignore: ensure .beads never leaks into any repo
RUN git config --system core.excludesFile /etc/gitignore-global && \
    echo ".beads" >> /etc/gitignore-global

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

# Initialize memory database if doc-sync is installed
DOCTOOL="/claude-env/home/.claude/skills/doc-sync/doctool.py"
if [ -f "$DOCTOOL" ]; then
    echo "Initializing memory database..."
    if ! python3 "$DOCTOOL" index init 2>&1; then
        echo "Warning: database init returned non-zero (may already exist)"
    fi
fi

# Initialize beads issue tracker if bd is installed
if command -v bd >/dev/null 2>&1; then
    BEADS_TARGET="/claude-env/repos/${REPO_ID}/.beads"
    mkdir -p "$BEADS_TARGET"

    # Write fish env config so BEADS_DIR is set in every shell
    FISH_CONF_DIR="/claude-env/home/.config/fish/conf.d"
    mkdir -p "$FISH_CONF_DIR"
    cat > "$FISH_CONF_DIR/beads-env.fish" << FISHENV
set -gx BEADS_DIR /claude-env/repos/${REPO_ID}/.beads
FISHENV

    export BEADS_DIR="$BEADS_TARGET"

    # Initialize if not already done
    if [ ! -f "$BEADS_TARGET/beads.db" ]; then
        echo "Initializing beads issue tracker..."
        if ! bd init --stealth --no-daemon 2>&1; then
            echo "Warning: beads init returned non-zero"
        fi
    else
        echo "Beads already initialized for this repo."
    fi

    # Configure beads for Claude Code integration (idempotent, safe every start)
    bd setup claude 2>&1 || echo "Warning: bd setup claude returned non-zero"
    bd config set beads.role maintainer 2>&1 || echo "Warning: bd config set role returned non-zero"
    bd config set daemon.enabled false 2>&1 || echo "Warning: bd config set daemon returned non-zero"
fi
SCRIPT
RUN chmod +x /usr/local/bin/setup-workspace-symlink.sh

# Switch to non-root user
USER claude
WORKDIR /workspace

# Configure fish with Starship
RUN mkdir -p ~/.config/fish && \
    cat > ~/.config/fish/config.fish << 'FISHCONFIG'
if status is-interactive
    # Commands to run in interactive sessions can go here
end
starship init fish | source

# Upgrade Claude Code to the latest version
function claude-upgrade
    echo "Upgrading Claude Code..."
    npm update -g @anthropic-ai/claude-code
    echo "Current version:"
    claude --version
end
FISHCONFIG

# Configure Starship with gruvbox-rainbow preset
RUN mkdir -p ~/.config && \
    starship preset gruvbox-rainbow -o ~/.config/starship.toml

# Copy fish config to /etc/skel for use with custom HOME directories
USER root
RUN mkdir -p /etc/skel/.config && \
    cp -r /home/claude/.config/fish /etc/skel/.config/ && \
    cp /home/claude/.config/starship.toml /etc/skel/.config/starship.toml

# Add init script to set up fish config in $HOME if missing
RUN cat > /etc/fish/conf.d/init-home.fish << 'INITSCRIPT'
# Initialize fish config in $HOME if not present
if test -n "$HOME"; and not test -f "$HOME/.config/fish/config.fish"
    mkdir -p "$HOME/.config/fish" 2>/dev/null; or true
    cp /etc/skel/.config/fish/config.fish "$HOME/.config/fish/config.fish" 2>/dev/null; or true
    cp /etc/skel/.config/starship.toml "$HOME/.config/starship.toml" 2>/dev/null; or true
end
INITSCRIPT

USER claude

# Environment variables
ENV CLAUDE_ENV_PATH=/claude-env
ENV ANTHROPIC_API_KEY_FILE=/claude-env/auth/api-key
ENV SHELL=/usr/bin/fish

# Entry point
ENTRYPOINT ["/usr/bin/fish"]
