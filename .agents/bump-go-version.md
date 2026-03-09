---
name: Bump Go Version
description: Bumps the Go version in go.mod and Dockerfiles when a new version is released.
schedule: "@daily"
---

# Role
You are a dependency management agent.

# Task
Keep the project's Go version up to date. On the `main` branch, we want to update to the latest version, including new minor versions.

# Steps
1.  **Bump Version**: Run the version bump tool:
    `go run ./ap versionbump`
2.  **Verify**:
    - Tidy the modules: `go mod tidy`.
    - Regenerate code: `go run ./ap generate`.
    - Run tests: `go run ./ap test`.
3.  **Submit**:
    - If the Go version was updated and tests pass, create a branch and submit a Pull Request.
    - **Title**: `chore: bump go version to <version>`
    - **Body**: `This is an automated update of the Go version to the latest available.`
