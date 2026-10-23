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

package scoped

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestHTTPClient(t *testing.T) {
	// Default when unset
	def := HTTPClient(t.Context())
	if def == nil {
		t.Fatal("HTTPClient(ctx) returned nil for unset context")
	}
	if def.Timeout <= 0 {
		t.Errorf("expected positive default timeout, got %v", def.Timeout)
	}

	// Usable default on nil context
	if nilCtx := HTTPClient(nil); nilCtx == nil || nilCtx.Timeout <= 0 {
		t.Errorf("expected usable client on nil context, got %v", nilCtx)
	}

	// Round trip when set
	custom := &http.Client{Timeout: 7 * time.Second}
	ctx := WithHTTPClient(t.Context(), custom)
	if got := HTTPClient(ctx); got != custom {
		t.Errorf("HTTPClient(ctx) = %v, want %v", got, custom)
	}
}

func TestLog(t *testing.T) {
	// Default when unset
	def := Log(t.Context())
	// Default logger must be usable and not panic on Info
	def.V(1).Info("test default log")

	// Usable default on nil context
	nilLog := Log(nil)
	nilLog.V(1).Info("test nil ctx log")

	// Round trip when set
	custom := logr.Discard()
	ctx := WithLog(t.Context(), custom)
	if got := Log(ctx); got != custom {
		t.Errorf("Log(ctx) = %v, want %v", got, custom)
	}
}

func TestNow(t *testing.T) {
	// Default when unset
	before := time.Now()
	got := Now(t.Context())
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("Now(ctx) = %v, expected between %v and %v", got, before, after)
	}

	// Usable default on nil context
	if nilNow := Now(nil); nilNow.IsZero() {
		t.Error("Now(nil) returned zero time")
	}

	// Round trip when set
	fixed := time.Date(2026, time.October, 23, 12, 0, 0, 0, time.UTC)
	ctx := WithNow(t.Context(), func() time.Time { return fixed })
	if gotFixed := Now(ctx); !gotFixed.Equal(fixed) {
		t.Errorf("Now(ctx) = %v, want %v", gotFixed, fixed)
	}
}
