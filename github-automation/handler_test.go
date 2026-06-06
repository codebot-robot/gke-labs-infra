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
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockTransport intercepts all default HTTP client requests.
type mockTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

func TestGenerateJWT(t *testing.T) {
	// Generate a mock RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	pkBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: pkBytes,
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	jwtStr, err := GenerateJWT("12345", pemBytes)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	if jwtStr == "" {
		t.Fatal("JWT string is empty")
	}

	parts := strings.Split(jwtStr, ".")
	if len(parts) != 3 {
		t.Fatalf("Expected 3 parts in JWT, got %d", len(parts))
	}
}

func TestAppHandler_ServeHTTP_InvalidSignature(t *testing.T) {
	handler := &AppHandler{
		AppID:         "12345",
		WebhookSecret: []byte("my-secret"),
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAppHandler_ProcessPR_FullCriteriaFlow(t *testing.T) {
	// 1. Generate test private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pkBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: pkBytes,
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	secret := []byte("secret")

	// 2. Mock transport responses
	var graphqlCalled bool
	transport := &mockTransport{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			// Mock Access Token Generation
			if strings.Contains(req.URL.Path, "/access_tokens") {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(strings.NewReader(`{"token": "mock-token"}`)),
				}, nil
			}

			// Mock GET PR API
			if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/pulls/10") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"number": 10,
						"state": "open",
						"draft": false,
						"node_id": "PR_NODE_ID_10",
						"head": { "sha": "headsha123" },
						"base": { "ref": "main" }
					}`)),
				}, nil
			}

			// Mock PR Reviews API
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/pulls/10/reviews") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`[
						{
							"state": "APPROVED",
							"user": { "login": "reviewer1" }
						}
					]`)),
				}, nil
			}

			// Mock Branch Protection API (NotFound to trigger fallback static checks)
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/branches/main/protection") {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{"message": "Not Found"}`)),
				}, nil
			}

			// Mock Check Runs API
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/commits/headsha123/check-runs") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"total_count": 1,
						"check_runs": [
							{
								"name": "ap-build",
								"status": "completed",
								"conclusion": "success"
							}
						]
					}`)),
				}, nil
			}

			// Mock Statuses API
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/commits/headsha123/statuses") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`[]`)),
				}, nil
			}

			// Mock GraphQL endpoint
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/graphql") {
				graphqlCalled = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"data": {}}`)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		},
	}

	// Substitute default client transport
	origTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = transport
	defer func() {
		http.DefaultClient.Transport = origTransport
	}()

	// Construct webhook request payload
	payload := []byte(`{
		"action": "submitted",
		"review": {
			"state": "approved"
		},
		"pull_request": {
			"number": 10
		},
		"repository": {
			"name": "gke-labs-infra",
			"owner": {
				"login": "gke-labs"
			}
		},
		"installation": {
			"id": 12345
		}
	}`)

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	handler := &AppHandler{
		AppID:          "12345",
		PrivateKey:     pemBytes,
		WebhookSecret:  secret,
		MinApprovals:   1,
		RequiredChecks: []string{"ap-build"},
		MergeMethod:    "SQUASH",
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	if !graphqlCalled {
		t.Error("GraphQL enablePullRequestAutoMerge mutation was not called")
	}
}

func TestAppHandler_ProcessPR_DraftSkipped(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	secret := []byte("secret")

	transport := &mockTransport{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/access_tokens") {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(strings.NewReader(`{"token": "mock-token"}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/pulls/10") {
				// PR is Draft
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"number": 10,
						"state": "open",
						"draft": true,
						"node_id": "PR_NODE_ID_10"
					}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		},
	}

	origTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = transport
	defer func() {
		http.DefaultClient.Transport = origTransport
	}()

	payload := []byte(`{
		"action": "submitted",
		"pull_request": {
			"number": 10
		},
		"repository": {
			"name": "gke-labs-infra",
			"owner": {
				"login": "gke-labs"
			}
		},
		"installation": {
			"id": 12345
		}
	}`)

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	handler := &AppHandler{
		AppID:         "12345",
		PrivateKey:    pemBytes,
		WebhookSecret: secret,
		MinApprovals:  1,
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestAppHandler_ProcessPR_InsufficientApprovals(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	secret := []byte("secret")

	transport := &mockTransport{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/access_tokens") {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(strings.NewReader(`{"token": "mock-token"}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/pulls/10") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"number": 10,
						"state": "open",
						"draft": false,
						"node_id": "PR_NODE_ID_10"
					}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/pulls/10/reviews") {
				// No approvals
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`[]`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		},
	}

	origTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = transport
	defer func() {
		http.DefaultClient.Transport = origTransport
	}()

	payload := []byte(`{
		"action": "submitted",
		"pull_request": {
			"number": 10
		},
		"repository": {
			"name": "gke-labs-infra",
			"owner": {
				"login": "gke-labs"
			}
		},
		"installation": {
			"id": 12345
		}
	}`)

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	handler := &AppHandler{
		AppID:         "12345",
		PrivateKey:    pemBytes,
		WebhookSecret: secret,
		MinApprovals:  1,
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestAppHandler_ProcessPR_FailedCI(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	secret := []byte("secret")

	transport := &mockTransport{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/access_tokens") {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(strings.NewReader(`{"token": "mock-token"}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/pulls/10") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"number": 10,
						"state": "open",
						"draft": false,
						"node_id": "PR_NODE_ID_10",
						"head": { "sha": "headsha123" },
						"base": { "ref": "main" }
					}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/pulls/10/reviews") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`[
						{ "state": "APPROVED", "user": { "login": "reviewer1" } }
					]`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/branches/main/protection") {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/commits/headsha123/check-runs") {
				// Failed check
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"total_count": 1,
						"check_runs": [
							{ "name": "ap-build", "status": "completed", "conclusion": "failure" }
						]
					}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/commits/headsha123/statuses") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`[]`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		},
	}

	origTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = transport
	defer func() {
		http.DefaultClient.Transport = origTransport
	}()

	payload := []byte(`{
		"action": "submitted",
		"pull_request": {
			"number": 10
		},
		"repository": {
			"name": "gke-labs-infra",
			"owner": {
				"login": "gke-labs"
			}
		},
		"installation": {
			"id": 12345
		}
	}`)

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	handler := &AppHandler{
		AppID:          "12345",
		PrivateKey:     pemBytes,
		WebhookSecret:  secret,
		MinApprovals:   1,
		RequiredChecks: []string{"ap-build"},
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestAppHandler_ProcessPR_WithBranchProtectionChecks(t *testing.T) {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	secret := []byte("secret")

	var graphqlCalled bool
	transport := &mockTransport{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/access_tokens") {
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(strings.NewReader(`{"token": "mock-token"}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/pulls/10") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"number": 10,
						"state": "open",
						"draft": false,
						"node_id": "PR_NODE_ID_10",
						"head": { "sha": "headsha123" },
						"base": { "ref": "main" }
					}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/pulls/10/reviews") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`[
						{ "state": "APPROVED", "user": { "login": "reviewer1" } }
					]`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/branches/main/protection") {
				// Return branch protection with required checks
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"required_status_checks": {
							"contexts": ["bp-check-legacy"],
							"checks": [
								{ "context": "bp-check-modern" }
							]
						}
					}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/commits/headsha123/check-runs") {
				// check run matches bp-check-modern
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`{
						"total_count": 1,
						"check_runs": [
							{ "name": "bp-check-modern", "status": "completed", "conclusion": "success" }
						]
					}`)),
				}, nil
			}
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/commits/headsha123/statuses") {
				// status matches bp-check-legacy
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`[
						{ "context": "bp-check-legacy", "state": "success" }
					]`)),
				}, nil
			}
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/graphql") {
				graphqlCalled = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"data": {}}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		},
	}

	origTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = transport
	defer func() {
		http.DefaultClient.Transport = origTransport
	}()

	payload := []byte(`{
		"action": "submitted",
		"pull_request": {
			"number": 10
		},
		"repository": {
			"name": "gke-labs-infra",
			"owner": {
				"login": "gke-labs"
			}
		},
		"installation": {
			"id": 12345
		}
	}`)

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	handler := &AppHandler{
		AppID:         "12345",
		PrivateKey:    pemBytes,
		WebhookSecret: secret,
		MinApprovals:  1,
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	if !graphqlCalled {
		t.Error("GraphQL was not triggered despite both legacy and modern branch protection checks passing")
	}
}
