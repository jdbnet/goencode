package queue

import (
	"log"
	"sync"

	"goencode/internal/db"
	"goencode/internal/encoder"
)

type Manager struct {
	FFmpegPath   string
	TempDir      string
	TriggerChan  chan struct{}
	StopChan     chan struct{}
	Broadcast    func(string, interface{})
	WebhookURL   string
	encoder      *encoder.FFmpegManager
	isProcessing bool
	mu           sync.Mutex
}

func NewManager(ffmpegPath, tempDir, webhookURL string, broadcast func(string, interface{})) *Manager {
	return &Manager{
		FFmpegPath:  ffmpegPath,
		TempDir:     tempDir,
		TriggerChan: make(chan struct{}, 1),
		StopChan:    make(chan struct{}),
		Broadcast:   broadcast,
		WebhookURL:  webhookURL,
		encoder:     encoder.NewManager(ffmpegPath),
	}
}

func (m *Manager) Start() {
	// Mark any interrupted processing jobs as failed
	if err := db.MarkProcessingAsFailed(); err != nil {
		log.Printf("Failed to mark interrupted jobs: %v", err)
	}

	go m.workerLoop()
	m.Trigger() // Initial trigger
}

func (m *Manager) Stop() {
	close(m.StopChan)
}

func (m *Manager) Trigger() {
	select {
	case m.TriggerChan <- struct{}{}:
	default:
	}
}

func (m *Manager) NotifySSE(event string, data interface{}) {
	if m.Broadcast != nil {
		m.Broadcast(event, data)
	}
}
