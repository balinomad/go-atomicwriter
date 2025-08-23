# README.md

[![Go](https://github.com/balinomad/go-atomicwriter/actions/workflows/go.yml/badge.svg)](https://github.com/balinomad/go-atomicwriter/actions/workflows/go.yml)

# go-atomicwriter

*A thread-safe `io.Writer` that lets you atomically swap the underlying writer. Built for high-concurrency applications.*

Perfect for use in:

  - Graceful log file rotation without downtime
  - Dynamic reconfiguration of application outputs (e.g., `stdout` to a file)
  - Testing and mocking writers in concurrent environments
  - Implementing failover strategies for network or file writers

## ✨ Features

  - **Thread-Safe Writes**: Allows multiple goroutines to write concurrently without data races.
  - **Atomic Swapping**: Atomically replaces the underlying writer, ensuring no writes are lost or interleaved during the transition.
  - **Graceful Sync**: Automatically calls `Sync()` or `Flush()` on the old writer before swapping to ensure all buffered data is persisted.
  - **High Performance**: Uses a `sync.RWMutex` to allow for concurrent, non-blocking writes. A swap operation acquires a full lock only for the brief moment it takes to switch writers.
  - **Interface-Driven**: Works with any `io.Writer` and automatically detects if the writer implements `Sync()` or `Flush()`.
  - **Minimal API**: Simple, clean, and dependency-free.

## 🚀 Usage

Here's a basic example of writing to one buffer, swapping it for another, and continuing to write.

```go
package main

import (
	"bytes"
	"fmt"
	"log"

	"github.com/balinomad/go-atomicwriter"
)

func main() {
	// 1. Start with an initial writer (e.g., a buffer).
	buf1 := &bytes.Buffer{}
	aw, err := atomicwriter.NewAtomicWriter(buf1)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Write some data to the initial writer.
	fmt.Fprintln(aw, "first message")

	// 3. Create a new writer to swap to.
	buf2 := &bytes.Buffer{}

	// 4. Atomically swap the underlying writer.
	// The old writer (buf1) is guaranteed to be flushed if it supports it.
	if err := aw.Swap(buf2); err != nil {
		log.Fatal(err)
	}

	// 5. All subsequent writes go to the new writer.
	fmt.Fprintln(aw, "second message")

	// Verify the output.
	fmt.Printf("Buffer 1 contains: %q\n", buf1.String())
	fmt.Printf("Buffer 2 contains: %q\n", buf2.String())
}

// Output:
// Buffer 1 contains: "first message\n"
// Buffer 2 contains: "second message\n"
```

## 📌 Installation

```bash
go get github.com/balinomad/go-atomicwriter@latest
```

## 📘 API Highlights

| Method                   | Description                                                                  |
| ------------------------ | ---------------------------------------------------------------------------- |
| `NewAtomicWriter(w)`     | Creates a new writer. Returns an error if `w` is `nil`.                      |
| `MustNewAtomicWriter(w)` | Creates a new writer. Panics if `w` is `nil`.                                |
| `Write(p)`               | Writes data to the current underlying writer. It is safe for concurrent use. |
| `Swap(w)`                | Atomically replaces the underlying writer, syncing the old one first.        |
| `Sync()`                 | Calls `Sync()` or `Flush()` on the current writer if supported.              |

## 🔧 Advanced Example: Logger Hot-Swapping

`AtomicWriter` is ideal for managing logger outputs. You can reconfigure logging targets—for example, from `stdout` to a file—at runtime without restarting your application.

```go
package main

import (
	"log"
	"os"
	"time"

	"github.com/balinomad/go-atomicwriter"
)

func main() {
	// Start by logging to standard output.
	aw := atomicwriter.MustNewAtomicWriter(os.Stdout)

	// Configure a standard logger to use our atomic writer.
	logger := log.New(aw, "app: ", log.LstdFlags)

	logger.Println("Application starting, logging to stdout.")

	// Simulate a runtime configuration change after 2 seconds.
	time.AfterFunc(2*time.Second, func() {
		logFile, err := os.Create("app.log")
		if err != nil {
			logger.Printf("Error creating log file: %v", err)
			return
		}

		// Swap the writer to the new log file.
		// os.Stdout has no Sync(), so the swap proceeds immediately.
		// If it were a buffered writer, Swap would ensure it's flushed.
		aw.Swap(logFile)

		logger.Println("Switched to file-based logging.")
		// Note: We don't need to call logger.SetOutput() again!
	})

	// Continue logging every second.
	for i := 1; i <= 4; i++ {
		logger.Printf("Log message #%d", i)
		time.Sleep(1 * time.Second)
	}

	logger.Println("Application finished.")
}
```

### Expected Output

**Console:**

```
app: 2025/08/24 01:24:01 Application starting, logging to stdout.
app: 2025/08/24 01:24:01 Log message #1
app: 2025/08/24 01:24:02 Log message #2
```

**File `app.log`:**

```
app: 2025/08/24 01:24:03 Switched to file-based logging.
app: 2025/08/24 01:24:03 Log message #3
app: 2025/08/24 01:24:04 Log message #4
app: 2025/08/24 01:24:05 Application finished.
```

## ⚖️ License

MIT License — see `LICENSE` file for details.