# github-automation

A Go-based GitHub App service that automates enqueuing approved and verified Pull Requests into the GitHub Merge Queue.

## How It Works

This service listens for GitHub Webhook events and validates them against a configured secret.

1. **Event Triggers**: The app listens for:
   - `pull_request_review`: Detects new approvals.
   - `check_run`, `check_suite`, `status`: Detects CI completion.
2. **Criteria Validation**:
   - Skips Draft PRs.
   - Ensures the PR has the required number of human approvals (defaults to `1`, configurable via `MIN_APPROVALS`).
   - Dynamically checks target branch protection rules to determine required status checks.
   - Matches both Check Runs (GitHub Actions) and Statuses to verify they all succeeded (`success` status/conclusion).
3. **Queue / Auto-merge Activation**:
   - Triggers the GitHub GraphQL `enablePullRequestAutoMerge` mutation to request auto-merge, which puts the PR in the Merge Queue automatically.

## Configuration

Configure the application using the following environment variables:

| Environment Variable | Description | Default |
| --- | --- | --- |
| `PORT` | Port for the webhook server to listen on. | `8080` |
| `GITHUB_APP_ID` | **Required**. The GitHub App ID. | |
| `GITHUB_PRIVATE_KEY` | The private key PEM content. | |
| `GITHUB_PRIVATE_KEY_PATH`| Path to the private key PEM file (alternative to `GITHUB_PRIVATE_KEY`). | |
| `WEBHOOK_SECRET` | **Required**. GitHub App Webhook secret for payload verification. | |
| `MIN_APPROVALS` | Minimum human approvals required to enqueue the PR. | `1` |
| `REQUIRED_CHECKS` | Comma-separated fallback check names if branch protection is not set. | |
| `MERGE_METHOD` | Merge method to select when enabling auto-merge (`SQUASH`, `MERGE`, or `REBASE`). | `SQUASH` |

## GitHub App Permissions

The GitHub App must be configured with the following permissions:

- **Pull requests**: Write (to trigger auto-merge/queue action)
- **Checks**: Read (to monitor check run status)
- **Statuses**: Read (to monitor commit statuses)
- **Metadata**: Read (required default)

Subscribe the app to the following webhook events:
- Check run
- Check suite
- Pull request review
- Status

## Running Locally

To run the application locally:

```bash
cd github-automation
export GITHUB_APP_ID="123456"
export GITHUB_PRIVATE_KEY_PATH="/path/to/key.pem"
export WEBHOOK_SECRET="my-secret"
go run .
```
