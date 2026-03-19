FROM alpine:3.21

# Base packages
RUN apk add --no-cache \
    bash \
    git \
    tmux \
    curl \
    openssh-client \
    nodejs \
    npm \
    python3

# Install Claude Code
RUN npm install -g @anthropic-ai/claude-code

# Clean up npm cache
RUN npm cache clean --force

# Set up a non-root user
RUN adduser -D -s /bin/bash claude
USER claude
WORKDIR /home/claude

# Default shell
ENV SHELL=/bin/bash
CMD ["/bin/bash"]
