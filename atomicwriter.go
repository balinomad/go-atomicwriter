package atomicwriter

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

// ErrNilWriter is returned when a nil writer is provided.
var ErrNilWriter = errors.New("writer cannot be nil")

// syncer is an internal interface used to detect Sync method.
type syncer interface {
	Sync() error
}

// flusher is an internal interface used to detect Flush methods.
type flusher interface {
	Flush() error
}

// writerHolder holds the writer and its sync function.
// It is a simple struct stored in an atomic.Value.
type writerHolder struct {
	w      io.Writer
	syncFn func() error
}

// newWriterHolder creates a new holder and determines the sync function.
func newWriterHolder(w io.Writer) *writerHolder {
	wh := &writerHolder{w: w}

	if s, ok := w.(syncer); ok {
		wh.syncFn = s.Sync
	} else if f, ok := w.(flusher); ok {
		wh.syncFn = f.Flush
	} else {
		// If neither is supported, syncFn is a no-op
		wh.syncFn = func() error { return nil }
	}

	return wh
}

// AtomicWriter is a thread-safe io.Writer that allows atomically
// swapping the underlying writer.
//
// It ensures that writes and swaps are serialized, preventing race conditions
// and ensuring that writes do not interleave with a swap operation. If the
// underlying writer implements a Sync() or Flush() method, it will be called
// on the old writer before a swap is completed.
type AtomicWriter struct {
	holder atomic.Value // Stores *writerHolder
	mu     sync.RWMutex // Protects writes and swaps
}

// Ensure AtomicWriter implements io.Writer.
var _ io.Writer = (*AtomicWriter)(nil)

// NewAtomicWriter creates an AtomicWriter with the given writer.
// It returns an error if the writer is nil.
func NewAtomicWriter(w io.Writer) (*AtomicWriter, error) {
	if w == nil {
		return nil, ErrNilWriter
	}

	aw := &AtomicWriter{}
	aw.holder.Store(newWriterHolder(w))
	return aw, nil
}

// MustNewAtomicWriter creates an AtomicWriter with the given writer.
// It panics if the writer is nil. This is useful for cases where a nil
// writer is considered a programmer error and should not be handled at runtime.
func MustNewAtomicWriter(w io.Writer) *AtomicWriter {
	aw, err := NewAtomicWriter(w)
	if err != nil {
		panic(err)
	}
	return aw
}

// Write writes to the current underlying writer.
// It acquires a read lock, allowing multiple writes to proceed concurrently.
// A Swap operation will block until all active writes are finished.
func (aw *AtomicWriter) Write(p []byte) (int, error) {
	aw.mu.RLock()
	defer aw.mu.RUnlock()

	holder := aw.holder.Load().(*writerHolder)
	return holder.w.Write(p)
}

// Swap replaces the underlying writer atomically.
// It acquires a write lock, blocking all new and ongoing Write calls.
// It first calls Sync() on the old writer if supported. If Sync fails,
// the swap is aborted, and the error is returned.
func (aw *AtomicWriter) Swap(w io.Writer) error {
	if w == nil {
		return ErrNilWriter
	}

	aw.mu.Lock()
	defer aw.mu.Unlock()

	// Sync the old writer before swapping
	oldHolder := aw.holder.Load().(*writerHolder)
	if err := oldHolder.syncFn(); err != nil {
		// Abort swap on sync failure
		return err
	}

	aw.holder.Store(newWriterHolder(w))
	return nil
}

// Sync calls Sync() (or Flush()) on the current writer if supported.
// It acquires a read lock to ensure the underlying writer is not swapped
// during the operation.
func (aw *AtomicWriter) Sync() error {
	aw.mu.RLock()
	defer aw.mu.RUnlock()
	holder := aw.holder.Load().(*writerHolder)
	return holder.syncFn()
}
