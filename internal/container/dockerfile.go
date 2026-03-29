package container

// GenerateDockerfile returns the Dockerfile content for the fusebox remote container.
// The image is based on nestybox/ubuntu-jammy-systemd-docker which provides
// systemd + Docker pre-configured for use with Sysbox runtime.
func GenerateDockerfile() string {
	return `FROM nestybox/ubuntu-jammy-systemd-docker

RUN apt-get update && apt-get install -y curl mosh && \
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install -y nodejs && \
    npm install -g @anthropic-ai/claude-code && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# fusebox binary will be copied in at container creation time
`
}
