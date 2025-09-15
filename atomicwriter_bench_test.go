package atomicwriter

import (
	"io"
	"sync"
	"testing"
)

// mutexWriter is a simple thread-safe writer using a sync.RWMutex.
// It serves as a baseline for performance comparison.
type mutexWriter struct {
	mu sync.RWMutex
	w  io.Writer
}

// Write writes to the underlying writer while holding a read lock.
// It returns the number of bytes written and any error encountered.
func (mw *mutexWriter) Write(p []byte) (int, error) {
	mw.mu.RLock()
	defer mw.mu.RUnlock()
	return mw.w.Write(p)
}

// Swap atomically replaces the underlying writer with the provided writer.
// It acquires a write lock and updates the writer, then releases the lock.
// This ensures that writes and swaps are serialized, preventing race conditions.
func (mw *mutexWriter) Swap(w io.Writer) {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.w = w
}

var testData = []byte("this is some benchmark test data")

// BenchmarkWrite compares the write performance of mutexWriter and AtomicWriter.
// It benchmarks a single goroutine writing to each writer and measures the
// throughput in bytes per second.
func BenchmarkWrite(b *testing.B) {
	b.Run("mutex_writer", func(b *testing.B) {
		mw := &mutexWriter{w: &blackholeWriter{}}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			mw.Write(testData)
		}
	})

	b.Run("atomic_writer", func(b *testing.B) {
		aw := MustNewAtomicWriter(&safeBuffer{})
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			aw.Write(testData)
		}
	})
}

// BenchmarkConcurrentWrite compares the write performance of AtomicWriter
// against a standard mutex-protected writer under high contention.
func BenchmarkConcurrentWrite(b *testing.B) {
	b.Run("mutex_writer", func(b *testing.B) {
		mw := &mutexWriter{w: &blackholeWriter{}}
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				mw.Write(testData)
			}
		})
	})
	b.Run("atomic_writer", func(b *testing.B) {
		aw := MustNewAtomicWriter(&blackholeWriter{})
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				aw.Write(testData)
			}
		})
	})
}

// BenchmarkSwap compares the performance of swapping the underlying writer.
func BenchmarkSwap(b *testing.B) {
	b.Run("mutex_writer", func(b *testing.B) {
		mw := &mutexWriter{w: &blackholeWriter{}}
		writers := make([]*safeBuffer, b.N)
		for i := range writers {
			writers[i] = &safeBuffer{}
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			mw.Swap(&blackholeWriter{})
		}
	})

	b.Run("atomic_writer", func(b *testing.B) {
		aw := MustNewAtomicWriter(&blackholeWriter{})
		writers := make([]*safeBuffer, b.N)
		for i := range writers {
			writers[i] = &safeBuffer{}
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			aw.Swap(&blackholeWriter{})
		}
	})
}

// BenchmarkConcurrentWriteAndSwap simulates a more realistic workload
// with concurrent writes and occasional swaps.
func BenchmarkConcurrentWriteAndSwap(b *testing.B) {
	b.Run("mutex_writer", func(b *testing.B) {
		mw := &mutexWriter{w: &blackholeWriter{}}
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				mw.Write(testData)
				// Perform a swap occasionally.
				if i%100 == 0 {
					mw.Swap(&blackholeWriter{})
				}
				i++
			}
		})
	})
	b.Run("atomic_writer", func(b *testing.B) {
		aw := MustNewAtomicWriter(&blackholeWriter{})
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				aw.Write(testData)
				// Perform a swap occasionally.
				if i%100 == 0 {
					aw.Swap(&blackholeWriter{})
				}
				i++
			}
		})
	})
	b.Run("atomic_writer_heavy_contention", func(b *testing.B) {
		aw := MustNewAtomicWriter(&safeBuffer{})
		buffers := make([]*safeBuffer, 10)
		for i := range buffers {
			buffers[i] = &safeBuffer{}
		}
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				aw.Write(testData)
				// Frequent swaps to create heavy contention
				if i%10 == 0 {
					aw.Swap(buffers[i%len(buffers)])
				}
				i++
			}
		})
	})
}
