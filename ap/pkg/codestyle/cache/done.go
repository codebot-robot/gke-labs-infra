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
	doneMagic      = "apdone\n"
	doneVersion    = uint16(1)
	doneRecordSize = uint16(40)
	hashSize       = 32
	flagsSize      = 8
	maxSteps       = 64
)

type pendingRecord struct {
	hash  [hashSize]byte
	flags uint64
}

// DoneIndex maintains a flat, sorted, fixed-record index of completed steps
// keyed by raw 32-byte SHA-256 hash. The record region is kept as a raw []byte slice
// and searched in place with zero heap allocations per lookup.
type DoneIndex struct {
	data      []byte
	records   []byte
	stepNames [maxSteps][]byte
	stepCount int
	pending   []pendingRecord
}

// NewDoneIndex creates an empty DoneIndex.
func NewDoneIndex() *DoneIndex {
	return &DoneIndex{}
}

// LoadDone loads a DoneIndex from a byte slice containing the contents of done.bin.
// Only the header is parsed; the records region is retained as a sub-slice.
// Exactly one allocation occurs when the caller receives the newly allocated *DoneIndex struct.
func LoadDone(data []byte) (*DoneIndex, error) {
	if len(data) < 13 {
		return nil, fmt.Errorf("done cache too small: %d bytes (minimum 13)", len(data))
	}
	if string(data[:7]) != doneMagic {
		return nil, fmt.Errorf("invalid done cache magic: %q", data[:7])
	}
	version := binary.LittleEndian.Uint16(data[7:9])
	if version != doneVersion {
		return nil, fmt.Errorf("unsupported done cache version: %d", version)
	}
	recordSize := binary.LittleEndian.Uint16(data[9:11])
	if recordSize != doneRecordSize {
		return nil, fmt.Errorf("unsupported done cache record size: %d (expected %d)", recordSize, doneRecordSize)
	}
	stepCount := binary.LittleEndian.Uint16(data[11:13])
	if stepCount > maxSteps {
		return nil, fmt.Errorf("step count %d exceeds maximum of %d", stepCount, maxSteps)
	}

	idx := &DoneIndex{
		data:      data,
		stepCount: int(stepCount),
	}

	offset := 13
	for i := 0; i < int(stepCount); i++ {
		if len(data) < offset+2 {
			return nil, fmt.Errorf("truncated done cache header reading step %d length", i)
		}
		nameLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if len(data) < offset+nameLen {
			return nil, fmt.Errorf("truncated done cache header reading step %d name", i)
		}
		idx.stepNames[i] = data[offset : offset+nameLen]
		offset += nameLen
	}

	records := data[offset:]
	if len(records)%int(doneRecordSize) != 0 {
		return nil, fmt.Errorf("corrupt done cache: record region length %d is not a multiple of %d", len(records), doneRecordSize)
	}
	idx.records = records

	return idx, nil
}

func (idx *DoneIndex) stepBit(step string) int {
	for i := 0; i < idx.stepCount; i++ {
		if string(idx.stepNames[i]) == step {
			return i
		}
	}
	return -1
}

func (idx *DoneIndex) getOrAssignStepBit(step string) (int, error) {
	if bit := idx.stepBit(step); bit >= 0 {
		return bit, nil
	}
	if idx.stepCount >= maxSteps {
		return -1, fmt.Errorf("cannot add step %q: maximum of %d steps reached", step, maxSteps)
	}
	bit := idx.stepCount
	idx.stepNames[bit] = []byte(step)
	idx.stepCount++
	return bit, nil
}

// IsDone returns true if the specified step has been completed for the given hash.
// Performs zero heap allocations.
func (idx *DoneIndex) IsDone(step, hash string) bool {
	var rawHash [hashSize]byte
	if !decodeHex32(&rawHash, hash) {
		return false
	}

	bit := idx.stepBit(step)
	if bit < 0 {
		return false
	}
	bitMask := uint64(1) << uint(bit)

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
			flags := binary.LittleEndian.Uint64(idx.records[offset+hashSize : offset+int(doneRecordSize)])
			return (flags & bitMask) != 0
		}
	}

	return false
}

// MarkDone marks a step as completed for the given hash in the pending list.
func (idx *DoneIndex) MarkDone(step, hash string) error {
	var rawHash [hashSize]byte
	if !decodeHex32(&rawHash, hash) {
		return fmt.Errorf("invalid hash %q: expected 64 hex characters", hash)
	}

	bit, err := idx.getOrAssignStepBit(step)
	if err != nil {
		return err
	}
	bitMask := uint64(1) << uint(bit)

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
			binary.LittleEndian.PutUint64(flagBytes[:], pending[j].flags)
			merged = append(merged, flagBytes[:]...)
			j++
		} else {
			// Hashes match: OR flags
			existFlags := binary.LittleEndian.Uint64(existingRecords[i+hashSize : i+int(doneRecordSize)])
			mergedFlags := existFlags | pending[j].flags
			merged = append(merged, existHash...)
			var flagBytes [flagsSize]byte
			binary.LittleEndian.PutUint64(flagBytes[:], mergedFlags)
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
		binary.LittleEndian.PutUint64(flagBytes[:], pending[j].flags)
		merged = append(merged, flagBytes[:]...)
	}

	// Compute header size: magic (7) + version (2) + recordSize (2) + stepCount (2)
	headerSize := 13
	for k := 0; k < idx.stepCount; k++ {
		headerSize += 2 + len(idx.stepNames[k])
	}

	fileBytes := make([]byte, 0, headerSize+len(merged))
	fileBytes = append(fileBytes, doneMagic...)

	var hBuf [2]byte
	binary.LittleEndian.PutUint16(hBuf[:], doneVersion)
	fileBytes = append(fileBytes, hBuf[:]...)

	binary.LittleEndian.PutUint16(hBuf[:], doneRecordSize)
	fileBytes = append(fileBytes, hBuf[:]...)

	binary.LittleEndian.PutUint16(hBuf[:], uint16(idx.stepCount))
	fileBytes = append(fileBytes, hBuf[:]...)

	for k := 0; k < idx.stepCount; k++ {
		binary.LittleEndian.PutUint16(hBuf[:], uint16(len(idx.stepNames[k])))
		fileBytes = append(fileBytes, hBuf[:]...)
		fileBytes = append(fileBytes, idx.stepNames[k]...)
	}

	fileBytes = append(fileBytes, merged...)
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
