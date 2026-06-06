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
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func main() {
	log.Println("Starting GitHub Merge Queue Automation App...")

	appID := os.Getenv("GITHUB_APP_ID")
	if appID == "" {
		log.Fatal("GITHUB_APP_ID environment variable is required")
	}

	// Support both inline private key content or file path
	privateKeyBytes := []byte(os.Getenv("GITHUB_PRIVATE_KEY"))
	if len(privateKeyBytes) == 0 {
		privateKeyPath := os.Getenv("GITHUB_PRIVATE_KEY_PATH")
		if privateKeyPath == "" {
			log.Fatal("Either GITHUB_PRIVATE_KEY or GITHUB_PRIVATE_KEY_PATH environment variable is required")
		}
		var err error
		privateKeyBytes, err = os.ReadFile(privateKeyPath)
		if err != nil {
			log.Fatalf("Failed to read GITHUB_PRIVATE_KEY_PATH from %s: %v", privateKeyPath, err)
		}
	}

	webhookSecretStr := os.Getenv("WEBHOOK_SECRET")
	if webhookSecretStr == "" {
		log.Fatal("WEBHOOK_SECRET environment variable is required")
	}
	webhookSecret := []byte(webhookSecretStr)

	minApprovals := 1
	if minApprovalsStr := os.Getenv("MIN_APPROVALS"); minApprovalsStr != "" {
		val, err := strconv.Atoi(minApprovalsStr)
		if err != nil {
			log.Fatalf("Invalid MIN_APPROVALS '%s': %v", minApprovalsStr, err)
		}
		minApprovals = val
	}

	var reqChecks []string
	if staticChecks := os.Getenv("REQUIRED_CHECKS"); staticChecks != "" {
		for _, s := range strings.Split(staticChecks, ",") {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				reqChecks = append(reqChecks, trimmed)
			}
		}
	}

	mergeMethod := "SQUASH"
	if customMergeMethod := os.Getenv("MERGE_METHOD"); customMergeMethod != "" {
		upper := strings.ToUpper(strings.TrimSpace(customMergeMethod))
		if upper == "SQUASH" || upper == "MERGE" || upper == "REBASE" {
			mergeMethod = upper
		} else {
			log.Fatalf("Invalid MERGE_METHOD '%s'. Must be one of: SQUASH, MERGE, REBASE.", customMergeMethod)
		}
	}

	handler := &AppHandler{
		AppID:          appID,
		PrivateKey:     privateKeyBytes,
		WebhookSecret:  webhookSecret,
		MinApprovals:   minApprovals,
		RequiredChecks: reqChecks,
		MergeMethod:    mergeMethod,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.Handle("/webhook", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	serverAddr := ":" + port
	log.Printf("Listening for webhooks on %s/webhook", serverAddr)
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
