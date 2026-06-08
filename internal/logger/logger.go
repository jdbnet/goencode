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
	log.SetFlags(0)
}

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
