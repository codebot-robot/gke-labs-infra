// Copyright 2026 Google LLC
package any

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAny(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer, "any_test")
}
