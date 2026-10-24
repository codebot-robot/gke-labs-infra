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

package cache

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func makeHexHash(prefix byte, id uint64) string {
	var raw [hashSize]byte
	raw[0] = prefix
	binary.BigEndian.PutUint64(raw[24:32], id)
	return hex.EncodeToString(raw[:])
}

// TestDoneIndex_HeaderRoundTrip tests header encoding and decoding, including adding a step.
func TestDoneIndex_HeaderRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "done.bin")

	idx := NewDoneIndex()
	hash1 := makeHexHash(0x11, 1)
	hash2 := makeHexHash(0x22, 2)

	if err := idx.MarkDone("gofmt", hash1); err != nil {
		t.Fatalf("MarkDone(gofmt) failed: %v", err)
	}
	if err := idx.MarkDone("tidy", hash1); err != nil {
		t.Fatalf("MarkDone(tidy) failed: %v", err)
	}
	if err := idx.MarkDone("vet", hash2); err != nil {
		t.Fatalf("MarkDone(vet) failed: %v", err)
	}

	if err := idx.Save(binPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	loaded, err := LoadDone(data)
	if err != nil {
		t.Fatalf("LoadDone failed: %v", err)
	}

	if loaded.stepCount != 3 {
		t.Fatalf("expected stepCount 3, got %d", loaded.stepCount)
	}
	if string(loaded.stepNames[0]) != "gofmt" {
		t.Errorf("step 0: expected 'gofmt', got %q", loaded.stepNames[0])
	}
	if string(loaded.stepNames[1]) != "tidy" {
		t.Errorf("step 1: expected 'tidy', got %q", loaded.stepNames[1])
	}
	if string(loaded.stepNames[2]) != "vet" {
		t.Errorf("step 2: expected 'vet', got %q", loaded.stepNames[2])
	}

	if !loaded.IsDone("gofmt", hash1) {
		t.Errorf("expected IsDone(gofmt, hash1) to be true")
	}
	if !loaded.IsDone("tidy", hash1) {
		t.Errorf("expected IsDone(tidy, hash1) to be true")
	}
	if loaded.IsDone("vet", hash1) {
		t.Errorf("expected IsDone(vet, hash1) to be false")
	}
	if !loaded.IsDone("vet", hash2) {
		t.Errorf("expected IsDone(vet, hash2) to be true")
	}

	// Add another step to existing cache
	hash3 := makeHexHash(0x33, 3)
	if err := loaded.MarkDone("custom-step", hash3); err != nil {
		t.Fatalf("MarkDone(custom-step) failed: %v", err)
	}
	if err := loaded.Save(binPath); err != nil {
		t.Fatalf("Save with added step failed: %v", err)
	}

	data2, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("ReadFile after added step failed: %v", err)
	}
	loaded2, err := LoadDone(data2)
	if err != nil {
		t.Fatalf("LoadDone after added step failed: %v", err)
	}

	if loaded2.stepCount != 4 {
		t.Fatalf("expected stepCount 4, got %d", loaded2.stepCount)
	}
	if string(loaded2.stepNames[3]) != "custom-step" {
		t.Errorf("step 3: expected 'custom-step', got %q", loaded2.stepNames[3])
	}
	if !loaded2.IsDone("custom-step", hash3) {
		t.Errorf("expected IsDone(custom-step, hash3) to be true")
	}
}

// TestDoneIndex_MergePending tests merging pending records into existing records:
// new hashes (before, middle, after) and existing hashes with a new bit.
func TestDoneIndex_MergePending(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "done.bin")

	// Create initial records
	hashA := makeHexHash(0x20, 1)
	hashC := makeHexHash(0x40, 3)

	idx := NewDoneIndex()
	if err := idx.MarkDone("gofmt", hashA); err != nil {
		t.Fatal(err)
	}
	if err := idx.MarkDone("gofmt", hashC); err != nil {
		t.Fatal(err)
	}
	if err := idx.Save(binPath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDone(data)
	if err != nil {
		t.Fatal(err)
	}

	// Pending additions:
	// hashStart: smaller than hashA (inserted at start)
	// hashA: existing hash, new bit "tidy"
	// hashB: between hashA and hashC
	// hashEnd: larger than hashC (inserted at end)
	hashStart := makeHexHash(0x10, 0)
	hashB := makeHexHash(0x30, 2)
	hashEnd := makeHexHash(0x50, 4)

	if err := loaded.MarkDone("tidy", hashStart); err != nil {
		t.Fatal(err)
	}
	if err := loaded.MarkDone("tidy", hashA); err != nil {
		t.Fatal(err)
	}
	if err := loaded.MarkDone("gofmt", hashB); err != nil {
		t.Fatal(err)
	}
	if err := loaded.MarkDone("tidy", hashEnd); err != nil {
		t.Fatal(err)
	}

	// Before save: pending lookups should succeed
	if !loaded.IsDone("tidy", hashStart) {
		t.Errorf("pending lookup hashStart failed")
	}
	if !loaded.IsDone("gofmt", hashA) || !loaded.IsDone("tidy", hashA) {
		t.Errorf("pending lookup hashA failed")
	}
	if !loaded.IsDone("gofmt", hashB) {
		t.Errorf("pending lookup hashB failed")
	}
	if !loaded.IsDone("tidy", hashEnd) {
		t.Errorf("pending lookup hashEnd failed")
	}

	if err := loaded.Save(binPath); err != nil {
		t.Fatal(err)
	}

	// Reload from disk
	savedData, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadDone(savedData)
	if err != nil {
		t.Fatal(err)
	}

	nRecords := len(reloaded.records) / int(doneRecordSize)
	if nRecords != 5 {
		t.Fatalf("expected 5 records, got %d", nRecords)
	}

	// Verify all records and flags
	if !reloaded.IsDone("tidy", hashStart) || reloaded.IsDone("gofmt", hashStart) {
		t.Errorf("hashStart flags incorrect")
	}
	// hashA has BOTH gofmt and tidy
	if !reloaded.IsDone("gofmt", hashA) || !reloaded.IsDone("tidy", hashA) {
		t.Errorf("hashA expected both gofmt and tidy to be done")
	}
	if !reloaded.IsDone("gofmt", hashB) || reloaded.IsDone("tidy", hashB) {
		t.Errorf("hashB flags incorrect")
	}
	if !reloaded.IsDone("gofmt", hashC) || reloaded.IsDone("tidy", hashC) {
		t.Errorf("hashC flags incorrect")
	}
	if !reloaded.IsDone("tidy", hashEnd) || reloaded.IsDone("gofmt", hashEnd) {
		t.Errorf("hashEnd flags incorrect")
	}
}

// TestDoneIndex_LookupHitMiss tests hit and miss behaviour at both ends of the range,
// within the range, for unknown steps, and for empty caches.
func TestDoneIndex_LookupHitMiss(t *testing.T) {
	// Empty index
	empty := NewDoneIndex()
	dummyHash := makeHexHash(0x10, 1)
	if empty.IsDone("gofmt", dummyHash) {
		t.Errorf("expected empty index lookup to return false")
	}

	h1 := makeHexHash(0x20, 10)
	h2 := makeHexHash(0x30, 20)
	h3 := makeHexHash(0x40, 30)

	idx := NewDoneIndex()
	if err := idx.MarkDone("gofmt", h1); err != nil {
		t.Fatal(err)
	}
	if err := idx.MarkDone("gofmt", h2); err != nil {
		t.Fatal(err)
	}
	if err := idx.MarkDone("gofmt", h3); err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "done.bin")
	if err := idx.Save(binPath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDone(data)
	if err != nil {
		t.Fatal(err)
	}

	// Hit at start
	if !loaded.IsDone("gofmt", h1) {
		t.Errorf("expected hit at start (h1)")
	}
	// Hit in middle
	if !loaded.IsDone("gofmt", h2) {
		t.Errorf("expected hit in middle (h2)")
	}
	// Hit at end
	if !loaded.IsDone("gofmt", h3) {
		t.Errorf("expected hit at end (h3)")
	}

	// Miss before start
	hBefore := makeHexHash(0x10, 1)
	if loaded.IsDone("gofmt", hBefore) {
		t.Errorf("expected miss before start")
	}
	// Miss between h1 and h2
	hBetween12 := makeHexHash(0x25, 15)
	if loaded.IsDone("gofmt", hBetween12) {
		t.Errorf("expected miss between h1 and h2")
	}
	// Miss between h2 and h3
	hBetween23 := makeHexHash(0x35, 25)
	if loaded.IsDone("gofmt", hBetween23) {
		t.Errorf("expected miss between h2 and h3")
	}
	// Miss after end
	hAfter := makeHexHash(0x50, 40)
	if loaded.IsDone("gofmt", hAfter) {
		t.Errorf("expected miss after end")
	}

	// Miss for unknown step
	if loaded.IsDone("unknown-step", h2) {
		t.Errorf("expected miss for unknown step")
	}

	// Miss for invalid hash strings
	if loaded.IsDone("gofmt", "short") {
		t.Errorf("expected miss for short hash")
	}
	if loaded.IsDone("gofmt", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdefZZ") {
		t.Errorf("expected miss for invalid hex chars")
	}
}

func build100kTestData() (binData []byte, jsonData []byte, sampleHashes []string, err error) {
	const count = 100_000
	sampleHashes = make([]string, count)

	idx := NewDoneIndex()
	doneJSON := map[string]map[string]bool{
		"gofmt": make(map[string]bool, count),
		"tidy":  make(map[string]bool, count),
	}

	for i := 0; i < count; i++ {
		var raw [hashSize]byte
		binary.BigEndian.PutUint64(raw[0:8], uint64(i))
		hStr := hex.EncodeToString(raw[:])
		sampleHashes[i] = hStr

		// gofmt on all, tidy on even entries
		if err := idx.MarkDone("gofmt", hStr); err != nil {
			return nil, nil, nil, err
		}
		doneJSON["gofmt"][hStr] = true
		if i%2 == 0 {
			if err := idx.MarkDone("tidy", hStr); err != nil {
				return nil, nil, nil, err
			}
			doneJSON["tidy"][hStr] = true
		}
	}

	binData, err = idx.Encode()
	if err != nil {
		return nil, nil, nil, err
	}

	jsonData, err = json.Marshal(doneJSON)
	if err != nil {
		return nil, nil, nil, err
	}

	return binData, jsonData, sampleHashes, nil
}

// TestDoneIndex_Allocs_100k verifies that LoadDone performs exactly one allocation
// and IsDone performs zero allocations on a 100k entry dataset.
func TestDoneIndex_Allocs_100k(t *testing.T) {
	binData, _, sampleHashes, err := build100kTestData()
	if err != nil {
		t.Fatalf("build100kTestData failed: %v", err)
	}

	// Load is one allocation
	var loaded *DoneIndex
	loadAllocs := testing.AllocsPerRun(100, func() {
		var lErr error
		loaded, lErr = LoadDone(binData)
		if lErr != nil {
			t.Fatalf("LoadDone failed: %v", lErr)
		}
	})
	if loadAllocs != 1.0 {
		t.Errorf("expected LoadDone to perform exactly 1 allocation, got %f", loadAllocs)
	}

	// Lookup is zero allocations
	midHash := sampleHashes[50_000]
	lookupAllocs := testing.AllocsPerRun(1000, func() {
		if !loaded.IsDone("gofmt", midHash) {
			t.Fatalf("expected IsDone(gofmt, midHash) to be true")
		}
	})
	if lookupAllocs != 0.0 {
		t.Errorf("expected IsDone to perform 0 allocations, got %f", lookupAllocs)
	}
}

func BenchmarkDoneBin_Load_100k(b *testing.B) {
	binData, _, _, err := build100kTestData()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx, err := LoadDone(binData)
		if err != nil || idx == nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDoneBin_Lookup_100k(b *testing.B) {
	binData, _, sampleHashes, err := build100kTestData()
	if err != nil {
		b.Fatal(err)
	}
	idx, err := LoadDone(binData)
	if err != nil {
		b.Fatal(err)
	}
	target := sampleHashes[50_000]

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !idx.IsDone("gofmt", target) {
			b.Fatal("lookup failed")
		}
	}
}

func BenchmarkDoneJSON_Load_100k(b *testing.B) {
	_, jsonData, _, err := build100kTestData()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var m map[string]map[string]bool
		if err := json.Unmarshal(jsonData, &m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDoneJSON_Lookup_100k(b *testing.B) {
	_, jsonData, sampleHashes, err := build100kTestData()
	if err != nil {
		b.Fatal(err)
	}
	var m map[string]map[string]bool
	if err := json.Unmarshal(jsonData, &m); err != nil {
		b.Fatal(err)
	}
	target := sampleHashes[50_000]

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !m["gofmt"][target] {
			b.Fatal("lookup failed")
		}
	}
}
