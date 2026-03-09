# autodeploy

`autodeploy` is a Kubernetes controller that monitors a git repository and automatically builds, tests, and deploys the code using `ap`.

## Overview

`autodeploy` is designed to run inside a Kubernetes cluster. It polls a target git repository for new commits. When a change is detected, it triggers a CI/CD pipeline by executing `ap` commands.

## Features (Planned)

*   **Git Polling**: Periodically checks for updates in a git repository.
*   **AP Integration**: Leverages `ap build`, `ap test`, and `ap deploy` for the CI/CD workflow.
*   **Pluggable Deployment Strategies**:
    *   Development: Deploy on every merge to the default branch.
    *   Staging/Production: Deploy based on tags or release branches.
*   **BuildKit Integration**: Uses a Docker BuildKit endpoint for building images without a local Docker daemon.
*   **Self-hosted Registry**: Includes or integrates with a self-hosted image registry for seamless image management.

## Getting Started

TODO: Add instructions on how to deploy `autodeploy` to a cluster.
