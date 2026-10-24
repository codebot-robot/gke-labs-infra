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
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
	"sort"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
)

const (
	doneHeaderSize = 64
	doneVersion    = uint16(1)
	doneRecordSize = uint16(36)
	hashSize       = 32
	flagsSize      = 4
)

var doneMagic = [8]byte{'a', 'p', 'd', 'o', 'n', 'e', '\n', 0}

// Step identifies a build/codestyle step tracked in done.bin flags.
type Step uint8

const (
	StepGofmt Step = iota // bit 0
	StepTidy              // bit 1
)

type pendingRecord struct {
	hash  [hashSize]byte
	flags uint32
}

// DoneIndex maintains a flat, sorted, fixed-record index of completed steps
// keyed by raw 32-byte SHA-256 hash. The record region is kept as a raw []byte slice
// starting at offset 64 (cache-line boundary) and searched in place with zero heap allocations per lookup.
type DoneIndex struct {
	data    []byte
	records []byte
	pending []pendingRecord
}

// NewDoneIndex creates an empty DoneIndex.
func NewDoneIndex() *DoneIndex {
	return &DoneIndex{}
}

// LoadDone loads a DoneIndex from a byte slice containing the contents of done.bin.
// Only the header is parsed; the records region is retained as a sub-slice starting at byte 64.
// Exactly one allocation occurs when the caller receives the newly allocated *DoneIndex struct.
func LoadDone(data []byte) (*DoneIndex, error) {
	if len(data) < doneHeaderSize {
		return nil, fmt.Errorf("done cache too small: %d bytes (minimum %d)", len(data), doneHeaderSize)
	}
	if string(data[:7]) != "apdone\n" {
		return nil, fmt.Errorf("invalid done cache magic: %q", data[:8])
	}
	version := binary.LittleEndian.Uint16(data[8:10])
	if version != doneVersion {
		return nil, fmt.Errorf("unsupported done cache version: %d", version)
	}
	recordSize := binary.LittleEndian.Uint16(data[10:12])
	if recordSize != doneRecordSize {
		return nil, fmt.Errorf("unsupported done cache record size: %d (expected %d)", recordSize, doneRecordSize)
	}
	recordCount := binary.LittleEndian.Uint32(data[12:16])
	records := data[doneHeaderSize:]
	if len(records) != int(recordCount)*int(doneRecordSize) {
		return nil, fmt.Errorf("done cache corrupt: record region length %d does not match count %d * size %d", len(records), recordCount, doneRecordSize)
	}

	return &DoneIndex{
		data:    data,
		records: records,
	}, nil
}

// IsDone returns true if the specified step has been completed for the given hash.
// Performs zero heap allocations.
func (idx *DoneIndex) IsDone(step Step, hash string) bool {
	if step >= 32 {
		return false
	}
	var rawHash [hashSize]byte
	if !decodeHex32(&rawHash, hash) {
		return false
	}

	bitMask := uint32(1) << step

	// Check pending list first
	for i := len(idx.pending) - 1; i >= 0; i-- {
		if idx.pending[i].hash == rawHash && (idx.pending[i].flags&bitMask) != 0 {
			return true
		}
	}

	// Binary search over sorted records region
	n := len(idx.records) / int(doneRecordSize)
	probeIdx := sort.Search(n, func(probe int) bool {
		offset := probe * int(doneRecordSize)
		return bytes.Compare(idx.records[offset:offset+hashSize], rawHash[:]) >= 0
	})
	if probeIdx < n {
		offset := probeIdx * int(doneRecordSize)
		if bytes.Equal(idx.records[offset:offset+hashSize], rawHash[:]) {
			flags := binary.LittleEndian.Uint32(idx.records[offset+hashSize : offset+int(doneRecordSize)])
			return (flags & bitMask) != 0
		}
	}

	return false
}

// MarkDone marks a step as completed for the given hash in the pending list.
func (idx *DoneIndex) MarkDone(step Step, hash string) error {
	if step >= 32 {
		return fmt.Errorf("step %d exceeds maximum 31", step)
	}
	var rawHash [hashSize]byte
	if !decodeHex32(&rawHash, hash) {
		return fmt.Errorf("invalid hash %q: expected 64 hex characters", hash)
	}

	bitMask := uint32(1) << step
	idx.pending = append(idx.pending, pendingRecord{
		hash:  rawHash,
		flags: bitMask,
	})
	return nil
}

// Encode merges pending records with existing records and encodes the entire done.bin format.
func (idx *DoneIndex) Encode() ([]byte, error) {
	// Sort pending list ascending by hash
	slices.SortFunc(idx.pending, func(a, b pendingRecord) int {
		return bytes.Compare(a.hash[:], b.hash[:])
	})

	// Coalesce duplicate hashes in pending
	pending := idx.pending
	if len(pending) > 1 {
		dedup := pending[:1]
		for i := 1; i < len(pending); i++ {
			if dedup[len(dedup)-1].hash == pending[i].hash {
				dedup[len(dedup)-1].flags |= pending[i].flags
			} else {
				dedup = append(dedup, pending[i])
			}
		}
		pending = dedup
	}

	// Two-pointer merge of existing records and sorted pending records
	existingRecords := idx.records
	merged := make([]byte, 0, len(existingRecords)+len(pending)*int(doneRecordSize))
	i := 0
	j := 0
	for i < len(existingRecords) && j < len(pending) {
		existHash := existingRecords[i : i+hashSize]
		pendHash := pending[j].hash[:]
		cmp := bytes.Compare(existHash, pendHash)
		if cmp < 0 {
			merged = append(merged, existingRecords[i:i+int(doneRecordSize)]...)
			i += int(doneRecordSize)
		} else if cmp > 0 {
			merged = append(merged, pendHash...)
			var flagBytes [flagsSize]byte
			binary.LittleEndian.PutUint32(flagBytes[:], pending[j].flags)
			merged = append(merged, flagBytes[:]...)
			j++
		} else {
			// Hashes match: OR flags
			existFlags := binary.LittleEndian.Uint32(existingRecords[i+hashSize : i+int(doneRecordSize)])
			mergedFlags := existFlags | pending[j].flags
			merged = append(merged, existHash...)
			var flagBytes [flagsSize]byte
			binary.LittleEndian.PutUint32(flagBytes[:], mergedFlags)
			merged = append(merged, flagBytes[:]...)
			i += int(doneRecordSize)
			j++
		}
	}
	if i < len(existingRecords) {
		merged = append(merged, existingRecords[i:]...)
	}
	for ; j < len(pending); j++ {
		merged = append(merged, pending[j].hash[:]...)
		var flagBytes [flagsSize]byte
		binary.LittleEndian.PutUint32(flagBytes[:], pending[j].flags)
		merged = append(merged, flagBytes[:]...)
	}

	totalRecords := len(merged) / int(doneRecordSize)
	fileBytes := make([]byte, doneHeaderSize+len(merged))
	copy(fileBytes[:8], doneMagic[:])
	binary.LittleEndian.PutUint16(fileBytes[8:10], doneVersion)
	binary.LittleEndian.PutUint16(fileBytes[10:12], doneRecordSize)
	binary.LittleEndian.PutUint32(fileBytes[12:16], uint32(totalRecords))
	// fileBytes[16:64] is zero padding to 64 bytes
	copy(fileBytes[doneHeaderSize:], merged)

	return fileBytes, nil
}

// Save writes the updated done.bin atomically using fileutils.WriteFileAtomic.
func (idx *DoneIndex) Save(path string) error {
	fileBytes, err := idx.Encode()
	if err != nil {
		return err
	}
	if err := fileutils.WriteFileAtomic(path, fileBytes, 0644); err != nil {
		return err
	}

	// Re-load in memory so subsequent lookups point into the freshly saved fileBytes
	updated, err := LoadDone(fileBytes)
	if err != nil {
		return err
	}
	*idx = *updated
	return nil
}

func decodeHex32(dst *[hashSize]byte, s string) bool {
	if len(s) != hashSize*2 {
		return false
	}
	for i := 0; i < hashSize; i++ {
		hi := fromHexChar(s[i*2])
		lo := fromHexChar(s[i*2+1])
		if hi == 255 || lo == 255 {
			return false
		}
		dst[i] = (hi << 4) | lo
	}
	return true
}

func fromHexChar(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	}
	return 255
}
