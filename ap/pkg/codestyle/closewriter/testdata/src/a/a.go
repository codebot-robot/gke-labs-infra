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

package a

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"testing"

	"k8s.io/klog/v2"
)

// --- Finding Cases ---

func testCreateDeferredLog(dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testCreateDeferredLog; return it so write failures are not silent`
			klog.V(2).Infof("failed to close %s: %v", dst, err)
		}
	}()
	return nil
}

func testCreateTempDeferredLog() error {
	tmp, err := os.CreateTemp("", "test")
	if err != nil {
		return err
	}
	defer func() {
		if err := tmp.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testCreateTempDeferredLog; return it so write failures are not silent`
			klog.Warningf("close temp: %v", err)
		}
	}()
	return nil
}

func testOpenFileWriteOnly(dst string) error {
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testOpenFileWriteOnly; return it so write failures are not silent`
			klog.Errorf("failed: %v", err)
		}
	}()
	return nil
}

func testOpenFileReadWrite(dst string) error {
	f, err := os.OpenFile(dst, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testOpenFileReadWrite; return it so write failures are not silent`
			klog.Warningf("failed: %v", err)
		}
	}()
	return nil
}

func testOpenFileAppend(dst string) error {
	f, err := os.OpenFile(dst, os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testOpenFileAppend; return it so write failures are not silent`
			klog.Warningf("failed: %v", err)
		}
	}()
	return nil
}

// Dropped-outright cases (plain defer or _ = close) are skipped by closewriter
// and left to droppederrors to avoid double-reporting.
func testDiscardedClose(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}

func testBlankIdentifierClose(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	return nil
}

func testStdlibLog(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testStdlibLog; return it so write failures are not silent`
			log.Printf("failed to close: %v", err)
		}
	}()
	return nil
}

func testSlog(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testSlog; return it so write failures are not silent`
			slog.Error("failed to close", "err", err)
		}
	}()
	return nil
}

func testStderrFprint(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testStderrFprint; return it so write failures are not silent`
			fmt.Fprintf(os.Stderr, "failed to close: %v\n", err)
		}
	}()
	return nil
}

func testUnknownProvenanceWithWriteString(f *os.File) error {
	defer func() {
		if err := f.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testUnknownProvenanceWithWriteString; return it so write failures are not silent`
			klog.Warningf("close: %v", err)
		}
	}()
	_, err := f.WriteString("hello")
	return err
}

func testUnknownProvenanceWithIoCopy(f *os.File, r io.Reader) error {
	defer func() {
		if err := f.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testUnknownProvenanceWithIoCopy; return it so write failures are not silent`
			klog.Warningf("close: %v", err)
		}
	}()
	_, err := io.Copy(f, r)
	return err
}

func testBufioWriterFlush() error {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	defer func() {
		if err := bw.Flush(); err != nil { // want `error from Flush\(\) on a writer is only logged in testBufioWriterFlush; return it so write failures are not silent`
			klog.Warningf("flush: %v", err)
		}
	}()
	return nil
}

func testGzipWriterClose() error {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	defer func() {
		if err := gw.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testGzipWriterClose; return it so write failures are not silent`
			klog.Warningf("close: %v", err)
		}
	}()
	return nil
}

func testZlibWriterClose() error {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	defer func() {
		if err := zw.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testZlibWriterClose; return it so write failures are not silent`
			klog.Warningf("close: %v", err)
		}
	}()
	return nil
}

func testTarWriterClose() error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer func() {
		if err := tw.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testTarWriterClose; return it so write failures are not silent`
			klog.Warningf("close: %v", err)
		}
	}()
	return nil
}

func testZipWriterClose() error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	defer func() {
		if err := zw.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testZipWriterClose; return it so write failures are not silent`
			klog.Warningf("close: %v", err)
		}
	}()
	return nil
}

func testPipeWriterClose() error {
	r, w := io.Pipe()
	_ = r
	defer func() {
		if err := w.Close(); err != nil { // want `error from Close\(\) on a writer is only logged in testPipeWriterClose; return it so write failures are not silent`
			klog.Warningf("close pipe: %v", err)
		}
	}()
	return nil
}

func testNonDeferredLog(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("data"); err != nil {
		if closeErr := f.Close(); closeErr != nil { // want `error from Close\(\) on a writer is only logged in testNonDeferredLog; return it so write failures are not silent`
			klog.Warningf("close: %v", closeErr)
		}
		return err
	}
	return f.Close()
}

func testLocalVarErrorsJoinOnlyLogged(dst string) error {
	out, createErr := os.Create(dst)
	if createErr != nil {
		return createErr
	}
	var err error
	defer func() {
		if cerr := out.Close(); cerr != nil { // want `error from Close\(\) on a writer is only logged in testLocalVarErrorsJoinOnlyLogged; return it so write failures are not silent`
			klog.Errorf("failed to close: %v", cerr)
			err = errors.Join(err, cerr)
		}
	}()
	return nil
}

// --- OK Cases ---

func testDirectReturn(dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	return out.Close()
}

func testReturnWrapped(dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close: %w", err)
	}
	return nil
}

func testErrorsJoinNamedResult(dst string) (retErr error) {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, out.Close())
	}()
	return nil
}

func testErrorsJoinVarNamedResult(dst string) (retErr error) {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			retErr = errors.Join(retErr, cerr)
		}
	}()
	return nil
}

func testNamedReturnAssigned(dst string) (err error) {
	out, createErr := os.Create(dst)
	if createErr != nil {
		return createErr
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return nil
}

func testNamedReturnWrapped(dst string) (err error) {
	out, createErr := os.Create(dst)
	if createErr != nil {
		return createErr
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close: %w", cerr)
		}
	}()
	return nil
}

func testLocalVarErrorsJoin(dst string) error {
	out, createErr := os.Create(dst)
	if createErr != nil {
		return createErr
	}
	var err error
	defer func() {
		if cerr := out.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()
	return err
}

func testLocalVarAssignedDirect(dst string) error {
	out, createErr := os.Create(dst)
	if createErr != nil {
		return createErr
	}
	var err error
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return err
}

func testLocalVarWrapped(dst string) error {
	out, createErr := os.Create(dst)
	if createErr != nil {
		return createErr
	}
	var err error
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close: %w", cerr)
		}
	}()
	return err
}

func testDefUseCycle(dst string) error {
	out, createErr := os.Create(dst)
	if createErr != nil {
		return createErr
	}
	var err error
	var lastErr error
	defer func() {
		if cerr := out.Close(); cerr != nil {
			lastErr = cerr
			err = lastErr
			lastErr = err
		}
	}()
	return err
}

func testPanicDirect(dst string) {
	out, _ := os.Create(dst)
	panic(out.Close())
}

func testPanicIf(dst string) {
	out, _ := os.Create(dst)
	if err := out.Close(); err != nil {
		panic(err)
	}
}

func testFatalLog(dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			log.Fatalf("failed to close: %v", err)
		}
	}()
	return nil
}

func testKlogFatal(dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			klog.Fatalf("failed to close: %v", err)
		}
	}()
	return nil
}

func testKlogExit(dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			klog.Exitf("failed to close: %v", err)
		}
	}()
	return nil
}

func testOsExit(dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close: %v\n", err)
			os.Exit(1)
		}
	}()
	return nil
}

func testDirectLogFatal(dst string) {
	out, _ := os.Create(dst)
	log.Fatal(out.Close())
}

func TestInTestFile(t *testing.T) {
	out, err := os.Create("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
	}()
}

func testCloseTwiceIdiom(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write([]byte("hello")); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func testCloseTwiceIdiomWithLoggingDefer(dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			klog.V(2).Infof("best-effort close: %v", err)
		}
	}()

	if _, err := f.Write([]byte("hello")); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func testReaderExemptionOpen(src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			klog.Warningf("failed to close reader: %v", err)
		}
	}()
	return nil
}

func testReaderExemptionOpenFileReadOnly(src string) error {
	f, err := os.OpenFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			klog.Warningf("failed to close read-only file: %v", err)
		}
	}()
	return nil
}

func testUnknownProvenanceReader(f *os.File) error {
	defer func() {
		if err := f.Close(); err != nil {
			klog.Warningf("failed to close file: %v", err)
		}
	}()
	var buf [10]byte
	_, err := f.Read(buf[:])
	return err
}

func testBufioFlushReturn() error {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	return bw.Flush()
}

func testGzipCloseReturn() error {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	return gw.Close()
}
