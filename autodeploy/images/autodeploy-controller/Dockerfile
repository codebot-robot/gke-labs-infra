# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ARG IMAGE_PREFIX=images.local
ARG IMAGE_TAG=latest

FROM ${IMAGE_PREFIX}/ap-golang:${IMAGE_TAG} AS ap-bin

FROM golang:1.26.2-trixie AS builder

WORKDIR /workspace
COPY . .

# Build autodeploy
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /usr/local/bin/autodeploy .

FROM golang:1.26.2-trixie
COPY --from=ap-bin /usr/local/bin/ap /usr/local/bin/ap
COPY --from=builder /usr/local/bin/autodeploy /usr/local/bin/autodeploy

# autodeploy needs git to clone repos and docker CLI to build images
RUN apt-get update && apt-get install -y git ca-certificates curl gnupg && \
    install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
    chmod a+r /etc/apt/keyrings/docker.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian trixie stable" | \
    tee /etc/apt/sources.list.d/docker.list > /dev/null && \
    apt-get update && apt-get install -y docker-ce-cli && \
    curl -LO --retry 5 "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$(dpkg --print-architecture)/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl && \
    curl -L --retry 5 "https://github.com/moby/buildkit/releases/download/v0.28.0/buildkit-v0.28.0.linux-$(dpkg --print-architecture).tar.gz" -o buildkit.tar.gz && \
    tar -xz -C /usr/local/bin --strip-components=1 bin/buildctl -f buildkit.tar.gz && \
    rm buildkit.tar.gz && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
ENTRYPOINT ["autodeploy"]
