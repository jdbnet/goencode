package logger

import (
	"container/ring"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

var (
	ringBuffer *ring.Ring
	bufferMu   sync.Mutex
	Broadcast  func(string, interface{})
)

type customWriter struct {
	stdout io.Writer
}

func (cw *customWriter) Write(p []byte) (n int, err error) {
	now := time.Now()
	msg := strings.TrimSpace(string(p))
	
	// Write to stdout with simple timestamp
	outStr := now.Format("2006/01/02 15:04:05 ") + msg + "\n"
	cw.stdout.Write([]byte(outStr))

	if msg == "" {
		return len(p), nil
	}

	entry := LogEntry{
		Timestamp: now,
		Message:   msg,
	}

	bufferMu.Lock()
	if ringBuffer == nil {
		ringBuffer = ring.New(100)
	}
	ringBuffer.Value = entry
	ringBuffer = ringBuffer.Next()
	bufferMu.Unlock()

	if Broadcast != nil {
		Broadcast("log", entry)
	}

	return len(p), nil
}

func Init(broadcast func(string, interface{})) {
	Broadcast = broadcast
	ringBuffer = ring.New(100)
	log.SetOutput(&customWriter{stdout: os.Stdout})
	// Remove timestamp from log format since we add it in LogEntry or we can let log package add it 
	// Wait, standard log adds timestamp. If standard log adds timestamp, our `msg` will contain the timestamp.
	// But `LogEntry` has a `Timestamp` field for the JSON.
	// It's cleaner to remove the standard log flags so `msg` is just the message, and let stdout print the plain msg?
	// But stdout wouldn't have timestamps then unless we format it ourselves.
	// Let's just remove log flags and format it ourselves for stdout!
	log.SetFlags(0)
}

// Since we removed log flags, we need to handle stdout formatting in Write if we want timestamps in stdout.
// Actually, let's keep the timestamp in stdout, we can modify customWriter.Write to prepend the timestamp to stdout.
// We'll update the writer in a moment if needed.

func GetRecentLogs() []LogEntry {
	bufferMu.Lock()
	defer bufferMu.Unlock()

	var logs []LogEntry
	if ringBuffer != nil {
		ringBuffer.Do(func(p interface{}) {
			if p != nil {
				logs = append(logs, p.(LogEntry))
			}
		})
	}
	return logs
}
