FROM ubuntu:24.04

# Avoid interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install minimal tools needed by sandbox operations
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        jq \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user with known UID/GID (matches container User: "1000:1000").
# Ubuntu 24.04 ships with a default 'ubuntu' user at 1000:1000 — remove it first.
RUN userdel -r ubuntu 2>/dev/null || true && \
    groupadd -f -g 1000 sandbox && \
    useradd -u 1000 -g 1000 -m -s /bin/bash sandbox

# Create directories that will be mounted as tmpfs at runtime.
# They must exist in the image for the tmpfs overlay to work correctly.
RUN mkdir -p /workspace /tmp && \
    chown sandbox:sandbox /workspace /tmp

WORKDIR /workspace
USER sandbox
