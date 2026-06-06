// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/google/go-github/v81/github"
)

// AppHandler implements http.Handler to process GitHub webhook events.
type AppHandler struct {
	AppID          string
	PrivateKey     []byte
	WebhookSecret  []byte
	MinApprovals   int
	RequiredChecks []string
	MergeMethod    string // SQUASH, MERGE, REBASE
}

// ServeHTTP handles the incoming Webhook HTTP requests.
func (h *AppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := github.ValidatePayload(r, h.WebhookSecret)
	if err != nil {
		log.Printf("Payload validation failed: %v", err)
		http.Error(w, "Invalid Signature", http.StatusUnauthorized)
		return
	}

	eventType := github.WebHookType(r)
	event, err := github.ParseWebHook(eventType, payload)
	if err != nil {
		log.Printf("Failed to parse webhook: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Dynamic installation ID extraction from the event payload
	var installationID int64
	if installation := getInstallation(event); installation != nil {
		installationID = installation.GetID()
	}

	if installationID == 0 {
		log.Printf("No installation ID found in event %s, skipping", eventType)
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("Received event %s for installation %d", eventType, installationID)

	ctx := r.Context()
	token, err := h.getInstallationToken(ctx, installationID)
	if err != nil {
		log.Printf("Failed to obtain installation token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create an authenticated GitHub client
	client := github.NewClient(http.DefaultClient).WithAuthToken(token)

	// Identify pull requests to process based on event type
	var processErrors []error
	switch e := event.(type) {
	case *github.PullRequestReviewEvent:
		pr := e.GetPullRequest()
		repo := e.GetRepo()
		if pr != nil && repo != nil {
			err := h.processPR(ctx, client, token, repo.GetOwner().GetLogin(), repo.GetName(), pr.GetNumber())
			if err != nil {
				processErrors = append(processErrors, err)
			}
		}

	case *github.CheckRunEvent:
		repo := e.GetRepo()
		checkRun := e.GetCheckRun()
		if repo != nil && checkRun != nil {
			sha := checkRun.GetHeadSHA()
			err := h.processPRsForSHA(ctx, client, token, repo.GetOwner().GetLogin(), repo.GetName(), sha)
			if err != nil {
				processErrors = append(processErrors, err)
			}
		}

	case *github.CheckSuiteEvent:
		repo := e.GetRepo()
		checkSuite := e.GetCheckSuite()
		if repo != nil && checkSuite != nil {
			sha := checkSuite.GetHeadSHA()
			err := h.processPRsForSHA(ctx, client, token, repo.GetOwner().GetLogin(), repo.GetName(), sha)
			if err != nil {
				processErrors = append(processErrors, err)
			}
		}

	case *github.StatusEvent:
		repo := e.GetRepo()
		if repo != nil {
			sha := e.GetSHA()
			err := h.processPRsForSHA(ctx, client, token, repo.GetOwner().GetLogin(), repo.GetName(), sha)
			if err != nil {
				processErrors = append(processErrors, err)
			}
		}

	default:
		log.Printf("Unhandled event type: %s", eventType)
	}

	if len(processErrors) > 0 {
		log.Printf("Encountered errors while processing: %v", processErrors)
		http.Error(w, "Process Errors Encountered", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Event Processed Successfully"))
}

func getInstallation(event interface{}) *github.Installation {
	switch e := event.(type) {
	case *github.PullRequestReviewEvent:
		return e.GetInstallation()
	case *github.CheckRunEvent:
		return e.GetInstallation()
	case *github.CheckSuiteEvent:
		return e.GetInstallation()
	case *github.StatusEvent:
		return e.GetInstallation()
	}
	return nil
}

// processPRsForSHA locates all open pull requests matching a commit SHA and processes them.
func (h *AppHandler) processPRsForSHA(ctx context.Context, client *github.Client, token string, owner, repo, sha string) error {
	log.Printf("Listing pull requests associated with SHA %s in %s/%s", sha, owner, repo)
	prs, _, err := client.PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return fmt.Errorf("failed to list PRs for SHA %s: %w", sha, err)
	}

	var errors []error
	for _, pr := range prs {
		if pr.GetState() != "open" {
			continue
		}
		err := h.processPR(ctx, client, token, owner, repo, pr.GetNumber())
		if err != nil {
			errors = append(errors, fmt.Errorf("PR #%d: %w", pr.GetNumber(), err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors processing PRs for SHA %s: %v", sha, errors)
	}
	return nil
}

// processPR evaluates a single Pull Request against auto-merge / queue eligibility criteria.
func (h *AppHandler) processPR(ctx context.Context, client *github.Client, token string, owner, repo string, prNum int) error {
	log.Printf("Evaluating PR %s/%s#%d for Merge Queue eligibility", owner, repo, prNum)

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, prNum)
	if err != nil {
		return fmt.Errorf("failed to get PR info: %w", err)
	}

	if pr.GetDraft() {
		log.Printf("PR %s/%s#%d is in Draft mode. Skipping.", owner, repo, prNum)
		return nil
	}

	if pr.GetState() != "open" {
		log.Printf("PR %s/%s#%d is in state %s. Skipping.", owner, repo, prNum, pr.GetState())
		return nil
	}

	// 1. Verify Human Approvals
	log.Printf("Fetching reviews for PR %s/%s#%d", owner, repo, prNum)
	reviews, _, err := client.PullRequests.ListReviews(ctx, owner, repo, prNum, &github.ListOptions{PerPage: 100})
	if err != nil {
		return fmt.Errorf("failed to list reviews: %w", err)
	}

	// Calculate latest review state for each unique reviewer
	latestReviewStates := make(map[string]string)
	for _, r := range reviews {
		user := r.GetUser().GetLogin()
		if user == "" {
			continue
		}
		// ListReviews returns reviews chronologically, so overwriting ensures we keep the latest.
		latestReviewStates[user] = r.GetState()
	}

	activeApprovals := 0
	hasChangesRequested := false
	for user, state := range latestReviewStates {
		switch state {
		case "APPROVED":
			activeApprovals++
		case "CHANGES_REQUESTED":
			hasChangesRequested = true
			log.Printf("Reviewer %s has outstanding changes requested on PR #%d", user, prNum)
		}
	}

	if hasChangesRequested {
		log.Printf("PR %s/%s#%d cannot be merged: there are outstanding changes requested.", owner, repo, prNum)
		return nil
	}

	if activeApprovals < h.MinApprovals {
		log.Printf("PR %s/%s#%d has %d active approval(s), but %d is required. Skipping.", owner, repo, prNum, activeApprovals, h.MinApprovals)
		return nil
	}

	log.Printf("PR %s/%s#%d meets approval criteria (Approvals: %d, Required: %d)", owner, repo, prNum, activeApprovals, h.MinApprovals)

	// 2. Determine Required Checks
	baseBranch := pr.GetBase().GetRef()
	log.Printf("Fetching branch protection settings for target branch '%s' on %s/%s", baseBranch, owner, repo)
	protection, resp, err := client.Repositories.GetBranchProtection(ctx, owner, repo, baseBranch)
	var reqChecks []string
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			log.Printf("Branch protection not found for branch '%s'. Falling back to configured static checks.", baseBranch)
			reqChecks = h.RequiredChecks
		} else {
			return fmt.Errorf("failed to fetch branch protection: %w", err)
		}
	} else if protection != nil && protection.RequiredStatusChecks != nil {
		if protection.RequiredStatusChecks.Contexts != nil {
			reqChecks = append(reqChecks, *protection.RequiredStatusChecks.Contexts...)
		}
		if protection.RequiredStatusChecks.Checks != nil {
			for _, check := range *protection.RequiredStatusChecks.Checks {
				if check.Context != "" {
					reqChecks = append(reqChecks, check.Context)
				}
			}
		}
	}

	// 3. Verify Required Checks Status
	sha := pr.GetHead().GetSHA()
	log.Printf("Evaluating status of required checks on HEAD SHA %s: %v", sha, reqChecks)

	// Fetch Check Runs and Statuses for the commit
	checkRuns, _, err := client.Checks.ListCheckRunsForRef(ctx, owner, repo, sha, &github.ListCheckRunsOptions{Filter: github.Ptr("all")})
	if err != nil {
		return fmt.Errorf("failed to fetch check runs: %w", err)
	}

	statuses, _, err := client.Repositories.ListStatuses(ctx, owner, repo, sha, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch statuses: %w", err)
	}

	// Match and verify status check results
	for _, required := range reqChecks {
		passed := false
		// Search Check Runs first
		for _, run := range checkRuns.CheckRuns {
			if run.GetName() == required {
				if run.GetConclusion() == "success" {
					passed = true
				} else {
					log.Printf("Required Check Run '%s' has conclusion '%s'", required, run.GetConclusion())
				}
				break
			}
		}

		if !passed {
			// Search Statuses
			for _, st := range statuses {
				if st.GetContext() == required {
					if st.GetState() == "success" {
						passed = true
					} else {
						log.Printf("Required Status Context '%s' has state '%s'", required, st.GetState())
					}
					break
				}
			}
		}

		if !passed {
			log.Printf("PR %s/%s#%d cannot be queued: required check '%s' has not succeeded.", owner, repo, prNum, required)
			return nil
		}
	}

	log.Printf("All required checks (%d) succeeded for PR %s/%s#%d.", len(reqChecks), owner, repo, prNum)

	// 4. Trigger auto-merge / add to merge queue via GraphQL mutation
	log.Printf("Triggering enablePullRequestAutoMerge mutation for PR %s/%s#%d", owner, repo, prNum)
	err = h.enableAutoMerge(ctx, token, pr.GetNodeID())
	if err != nil {
		return fmt.Errorf("failed to enable auto-merge: %w", err)
	}

	log.Printf("Successfully enabled auto-merge / queued PR %s/%s#%d!", owner, repo, prNum)
	return nil
}

// enableAutoMerge triggers the enablePullRequestAutoMerge GraphQL mutation to enqueue the PR.
func (h *AppHandler) enableAutoMerge(ctx context.Context, token string, prNodeID string) error {
	queryStr := `mutation($input: EnablePullRequestAutoMergeInput!) {
		enablePullRequestAutoMerge(input: $input) {
			pullRequest {
				id
				autoMergeRequest {
					enabledAt
					enabledBy {
						login
					}
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"pullRequestId": prNodeID,
			"mergeMethod":   h.MergeMethod,
		},
	}

	requestPayload := map[string]interface{}{
		"query":     queryStr,
		"variables": variables,
	}

	jsonData, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal GraphQL payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gke-labs-infra-github-automation")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("graphql HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql returned status %s: %s", resp.Status, string(respBody))
	}

	// Parse for errors inside JSON response payload (GraphQL specific error reporting)
	var graphqlResp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &graphqlResp); err == nil && len(graphqlResp.Errors) > 0 {
		var errMsgs []string
		for _, e := range graphqlResp.Errors {
			errMsgs = append(errMsgs, e.Message)
		}
		return fmt.Errorf("graphql errors: %s", strings.Join(errMsgs, "; "))
	}

	return nil
}

// getInstallationToken requests an installation access token using signed App JWT.
func (h *AppHandler) getInstallationToken(ctx context.Context, installationID int64) (string, error) {
	jwtStr, err := GenerateJWT(h.AppID, h.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate App JWT: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create access token request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtStr)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gke-labs-infra-github-automation")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to fetch installation token, status %s: %s", resp.Status, string(respBody))
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse access token response: %w", err)
	}

	return tokenResp.Token, nil
}
