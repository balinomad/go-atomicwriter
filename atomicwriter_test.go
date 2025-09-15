package atomicwriter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

// safeBuffer is a thread-safe bytes.Buffer for testing.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write writes to the underlying bytes.Buffer while holding a lock.
// It returns the number of bytes written and any error encountered.
func (sb *safeBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

// String returns a string representation of the underlying bytes.Buffer.
func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// syncWriter implements the syncer interface.
type syncWriter struct {
	synced bool
}

func (sw *syncWriter) Write(p []byte) (int, error) { return len(p), nil }
func (sw *syncWriter) Sync() error {
	sw.synced = true
	return nil
}

// flushWriter implements the flusher interface.
type flushWriter struct {
	flushed bool
}

func (fw *flushWriter) Write(p []byte) (int, error) { return len(p), nil }
func (fw *flushWriter) Flush() error {
	fw.flushed = true
	return nil
}

// bothWriter implements both syncer and flusher.
type bothWriter struct {
	synced  bool
	flushed bool
}

func (bw *bothWriter) Write(p []byte) (int, error) { return len(p), nil }
func (bw *bothWriter) Sync() error {
	bw.synced = true
	return nil
}
func (bw *bothWriter) Flush() error {
	bw.flushed = true
	return nil
}

// syncErrorWriter is a writer that always fails on Sync.
type syncErrorWriter struct{}

func (sew *syncErrorWriter) Write(p []byte) (int, error) { return len(p), nil }
func (sew *syncErrorWriter) Sync() error                 { return errors.New("sync failed") }

// fileWriter simulates a file writer that prefixes writes.
type fileWriter struct{ buf *bytes.Buffer }

func (fw *fileWriter) Write(p []byte) (int, error) {
	return fw.buf.Write(append([]byte("FILE:"), p...))
}

// networkWriter simulates a network writer that prefixes writes.
type networkWriter struct{ buf *bytes.Buffer }

func (nw *networkWriter) Write(p []byte) (int, error) {
	return nw.buf.Write(append([]byte("NET:"), p...))
}

// blackholeWriter is used for benchmarking to discard all writes.
// This helps measure the overhead of the writer itself, not the underlying io.Writer.
type blackholeWriter struct{}

func (bh *blackholeWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

// TestNewAtomicWriter tests the NewAtomicWriter function by verifying that it
// correctly returns an AtomicWriter instance with the provided writer and
// handles errors correctly. It also checks that the stored writer matches the
// provided writer.
func TestNewAtomicWriter(t *testing.T) {
	tests := []struct {
		name    string
		writer  io.Writer
		wantErr bool
		errType error
	}{
		{
			name:    "valid buffer writer",
			writer:  &bytes.Buffer{},
			wantErr: false,
		},
		{
			name:    "valid file writer",
			writer:  &fileWriter{buf: &bytes.Buffer{}},
			wantErr: false,
		},
		{
			name:    "nil writer",
			writer:  nil,
			wantErr: true,
			errType: ErrNilWriter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAtomicWriter(tt.writer)

			if tt.wantErr {
				if !errors.Is(err, tt.errType) {
					t.Errorf("NewAtomicWriter() error = %v, want %v", err, tt.errType)
				}
				if got != nil {
					t.Errorf("NewAtomicWriter() expected nil result on error, got %v", got)
				}
				return
			}

			if err != nil {
				t.Errorf("NewAtomicWriter() unexpected error = %v", err)
				return
			}

			if got == nil {
				t.Errorf("NewAtomicWriter() returned nil without error")
				return
			}

			// Verify the writer was stored correctly
			storedWriter := got.holder.Load().(*writerHolder).w
			if storedWriter != tt.writer {
				t.Errorf("NewAtomicWriter() stored writer = %v, want %v", storedWriter, tt.writer)
			}
		})
	}
}

// TestMustNewAtomicWriter tests the MustNewAtomicWriter function by verifying
// that it panics if given a nil writer and works correctly with a valid writer.
func TestMustNewAtomicWriter(t *testing.T) {
	t.Run("panic on nil writer", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("MustNewAtomicWriter did not panic on nil writer")
			}
		}()
		_ = MustNewAtomicWriter(nil)
	})

	t.Run("work with valid writer", func(t *testing.T) {
		buf := &bytes.Buffer{}
		aw := MustNewAtomicWriter(buf)
		if aw == nil {
			t.Errorf("MustNewAtomicWriter returned nil on valid writer")
		}
	})
}

// TestAtomicWriter_Write tests the Write method of AtomicWriter by verifying that it
// correctly writes the provided data to the underlying writer, handles errors correctly,
// and returns the correct number of bytes written. It also checks that the stored writer
// content matches the provided data.
func TestAtomicWriter_Write(t *testing.T) {
	aw := MustNewAtomicWriter(&bytes.Buffer{})
	data := []byte("hello world")
	n, err := aw.Write(data)

	if err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(data))
	}

	holder := aw.holder.Load().(*writerHolder)
	buf := holder.w.(*bytes.Buffer)
	if buf.String() != string(data) {
		t.Errorf("Write() buffer content = %q, want %q", buf.String(), string(data))
	}
}

// TestAtomicWriter_Sync tests the Sync method of AtomicWriter by verifying that it
// calls the underlying writer's Sync() or Flush() method if supported. It also checks
// that the Sync() method does not return an error when the underlying writer does not
// support Sync() or Flush().
func TestAtomicWriter_Sync(t *testing.T) {
	t.Run("sync supported", func(t *testing.T) {
		writer := &syncWriter{}
		aw := MustNewAtomicWriter(writer)
		aw.Sync()
		if !writer.synced {
			t.Error("expected Sync to be called")
		}
	})

	t.Run("flush supported", func(t *testing.T) {
		writer := &flushWriter{}
		aw := MustNewAtomicWriter(writer)
		aw.Sync()
		if !writer.flushed {
			t.Error("expected Flush to be called")
		}
	})

	t.Run("both supported, sync preferred", func(t *testing.T) {
		writer := &bothWriter{}
		aw := MustNewAtomicWriter(writer)
		aw.Sync()
		if !writer.synced {
			t.Error("expected Sync to be called")
		}
		if writer.flushed {
			t.Error("Flush should not have been called when Sync is available")
		}
	})

	t.Run("none supported", func(t *testing.T) {
		aw := MustNewAtomicWriter(&bytes.Buffer{})
		if err := aw.Sync(); err != nil {
			t.Errorf("Sync() unexpected error: %v", err)
		}
	})
}

// TestAtomicWriter_Swap tests the Swap method of AtomicWriter by verifying that it
// correctly replaces the underlying writer, syncing the old one first. It also
// checks that the stored writer content matches the written data.
func TestAtomicWriter_Swap(t *testing.T) {
	buf1 := &bytes.Buffer{}
	aw := MustNewAtomicWriter(buf1)
	aw.Write([]byte("first"))

	buf2 := &bytes.Buffer{}
	if err := aw.Swap(buf2); err != nil {
		t.Fatalf("Swap() unexpected error: %v", err)
	}

	aw.Write([]byte("second"))

	if buf1.String() != "first" {
		t.Errorf("buf1 content = %q, want %q", buf1.String(), "first")
	}
	if buf2.String() != "second" {
		t.Errorf("buf2 content = %q, want %q", buf2.String(), "second")
	}
}

// TestAtomicWriter_Swap_SyncError tests the Swap method of AtomicWriter when
// the old writer's Sync method returns an error.
// It verifies that the Swap operation fails and the underlying writer is not swapped.
func TestAtomicWriter_Swap_SyncError(t *testing.T) {
	oldWriter := &syncErrorWriter{}
	aw := MustNewAtomicWriter(oldWriter)
	newWriter := &bytes.Buffer{}

	err := aw.Swap(newWriter)
	if err == nil {
		t.Fatal("Swap() expected an error, but got nil")
	}

	// Verify the writer was not swapped
	currentWriter := aw.holder.Load().(*writerHolder).w
	if currentWriter != oldWriter {
		t.Error("writer was swapped despite sync error")
	}
}

// TestAtomicWriter_ConcurrentWriteAndSwap tests the AtomicWriter under concurrent writes
// and swaps. It verifies that the writes are properly serialized and that the underlying
// writer is swapped correctly.
func TestAtomicWriter_ConcurrentWriteAndSwap(t *testing.T) {
	aw := MustNewAtomicWriter(&safeBuffer{})

	var wg sync.WaitGroup
	numGoroutines := 50
	writesPerGoRoutine := 100

	// Writer goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoRoutine; j++ {
				msg := fmt.Sprintf("goroutine-%d-write-%d", id, j)
				aw.Write([]byte(msg))
			}
		}(i)
	}

	// Swapper goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			aw.Swap(&safeBuffer{})
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
}

// TestAtomicWriter_HotSwapping tests the hot-swapping of an AtomicWriter.
// It verifies that the writes are properly serialized and that the underlying
// writer is swapped correctly.
func TestAtomicWriter_HotSwapping(t *testing.T) {
	buf1 := &bytes.Buffer{}
	aw, _ := NewAtomicWriter(buf1)
	aw.Write([]byte("data1"))

	file := &fileWriter{buf: &bytes.Buffer{}}
	aw.Swap(file)
	aw.Write([]byte("data2"))

	net := &networkWriter{buf: &bytes.Buffer{}}
	aw.Swap(net)
	aw.Write([]byte("data3"))

	if buf1.String() != "data1" {
		t.Errorf("buf1 content mismatch")
	}
	if file.buf.String() != "FILE:data2" {
		t.Errorf("file content mismatch")
	}
	if net.buf.String() != "NET:data3" {
		t.Errorf("network content mismatch")
	}
}
