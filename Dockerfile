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

# Switch to non-root user
USER claude
WORKDIR /workspace

# Environment variables
ENV CLAUDE_ENV_PATH=/claude-env
ENV ANTHROPIC_API_KEY_FILE=/claude-env/auth/api-key

# Entry point
ENTRYPOINT ["/bin/bash"]
