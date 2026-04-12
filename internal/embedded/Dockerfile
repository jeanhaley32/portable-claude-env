FROM node:20-slim

ARG TARGETARCH

LABEL maintainer="jeanhaley32"
LABEL description="Claude Capsule workspace environment"

# Install system dependencies including fish, fonts, and Python
RUN apt-get update && apt-get install -y \
    git \
    curl \
    jq \
    ripgrep \
    fish \
    sudo \
    fontconfig \
    unzip \
    python3 \
    make \
    openssh-client \
    fd-find \
    bat \
    fzf \
    && rm -rf /var/lib/apt/lists/* \
    && ln -s /usr/bin/batcat /usr/local/bin/bat 2>/dev/null || true \
    && ln -s /usr/bin/fdfind /usr/local/bin/fd 2>/dev/null || true

# Install GitHub CLI (pin version for reproducible builds)
RUN curl -fLo /tmp/gh.deb "https://github.com/cli/cli/releases/download/v2.89.0/gh_2.89.0_linux_${TARGETARCH}.deb" \
    && dpkg -i /tmp/gh.deb \
    && rm /tmp/gh.deb

# Install uv (fast Python package manager)
RUN curl -LsSf https://astral.sh/uv/0.11.6/install.sh | env INSTALLER_NO_MODIFY_PATH=1 sh \
    && mv /root/.local/bin/uv /usr/local/bin/ \
    && mv /root/.local/bin/uvx /usr/local/bin/

# Install Python dev tools (isolated tool installs, symlinked to /usr/local/bin)
ENV UV_TOOL_DIR=/opt/uv-tools
ENV UV_TOOL_BIN_DIR=/usr/local/bin
RUN uv tool install ruff \
    && uv tool install basedpyright \
    && uv tool install pre-commit

# Install Pulumi CLI
RUN curl -fsSL https://get.pulumi.com | sh \
    && mv /root/.pulumi/bin/* /usr/local/bin/ \
    && rm -rf /root/.pulumi

# Install yq (YAML processor, pin version for reproducible builds)
RUN curl -fLo /usr/local/bin/yq "https://github.com/mikefarah/yq/releases/download/v4.52.5/yq_linux_${TARGETARCH}" \
    && chmod +x /usr/local/bin/yq

# Install AWS CLI v2 (map TARGETARCH to AWS naming convention)
RUN if [ "$TARGETARCH" = "arm64" ]; then AWS_ARCH="aarch64"; else AWS_ARCH="x86_64"; fi \
    && curl -fLo /tmp/awscliv2.zip "https://awscli.amazonaws.com/awscli-exe-linux-${AWS_ARCH}.zip" \
    && unzip /tmp/awscliv2.zip -d /tmp \
    && /tmp/aws/install \
    && rm -rf /tmp/aws /tmp/awscliv2.zip

# Install Nerd Font (FiraCode)
RUN mkdir -p /usr/local/share/fonts && \
    curl -fLo /tmp/FiraCode.zip https://github.com/ryanoasis/nerd-fonts/releases/latest/download/FiraCode.zip && \
    unzip /tmp/FiraCode.zip -d /usr/local/share/fonts/FiraCode && \
    rm /tmp/FiraCode.zip && \
    fc-cache -fv

# Install Starship prompt
RUN curl -sS https://starship.rs/install.sh | sh -s -- -y

# Install Claude Code CLI (pin version for reproducible builds)
RUN npm install -g @anthropic-ai/claude-code@2.1.104

# Install Beads issue tracker (vendored binary from jeanhaley32/beads fork)
RUN curl -fLo /tmp/beads.tar.gz \
    "https://github.com/jeanhaley32/beads/releases/download/v0.55.4/beads_0.55.4_linux_${TARGETARCH}.tar.gz" \
    && tar xzf /tmp/beads.tar.gz -C /usr/local/bin/ bd \
    && chmod +x /usr/local/bin/bd \
    && rm /tmp/beads.tar.gz

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

    # Write bash profile for non-fish shells
    PROFILE="/claude-env/home/.profile"
    if ! grep -q 'BEADS_DIR' "$PROFILE" 2>/dev/null; then
        echo "export BEADS_DIR=/claude-env/repos/${REPO_ID}/.beads" >> "$PROFILE"
    fi

    # Initialize if not already done
    if [ ! -f "$BEADS_TARGET/beads.db" ]; then
        echo "Initializing beads issue tracker..."
        if ! bd init --stealth 2>&1; then
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
ENV BEADS_AUTO_START_DAEMON=false
ENV SHELL=/usr/bin/fish

# Entry point
ENTRYPOINT ["/usr/bin/fish"]
