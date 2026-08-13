package queue

import (
	"context"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"goencode/internal/db"
	"goencode/internal/encoder"
)

type Manager struct {
	FFmpegPath  string
	TempDir     string
	TriggerChan chan struct{}
	StopChan    chan struct{}
	Broadcast   func(string, interface{})
	WebhookURL  string
	encoder     *encoder.FFmpegManager

	mu           sync.Mutex
	isProcessing bool
	currentJobID int
	currentCmd   *exec.Cmd
	currentTemps []string
	jobCancel    context.CancelFunc
	shuttingDown bool
	doneChan     chan struct{}
	stopOnce     sync.Once
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
		doneChan:    make(chan struct{}),
	}
}

func (m *Manager) Start() {
	if err := db.MarkProcessingAsFailed(); err != nil {
		log.Printf("Failed to mark interrupted jobs: %v", err)
	}

	go func() {
		defer close(m.doneChan)
		m.workerLoop()
	}()
	m.Trigger()
}

func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		m.mu.Lock()
		m.shuttingDown = true
		cmd := m.currentCmd
		cancel := m.jobCancel
		m.mu.Unlock()

		if cancel != nil {
			cancel()
		}
		interruptCmd(cmd)
		go escalateKill(cmd, m.cmdStillCurrent)

		close(m.StopChan)

		select {
		case <-m.doneChan:
		case <-time.After(15 * time.Second):
			log.Printf("Queue worker did not stop in time")
			killCmd(cmd)
			select {
			case <-m.doneChan:
			case <-time.After(2 * time.Second):
			}
		}
	})
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

func (m *Manager) Cancel(id int) error {
	m.mu.Lock()
	if m.currentJobID == id {
		cmd := m.currentCmd
		cancel := m.jobCancel
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		interruptCmd(cmd)
		go escalateKill(cmd, m.cmdStillCurrent)
		log.Printf("Cancelling in-progress job %d", id)
		return nil
	}
	m.mu.Unlock()
	return db.DeleteJob(id)
}

func (m *Manager) beginJob(id int) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shuttingDown {
		cancel()
		return ctx
	}
	m.currentJobID = id
	m.currentCmd = nil
	m.currentTemps = nil
	m.jobCancel = cancel
	return ctx
}

func (m *Manager) setCurrentCmd(cmd *exec.Cmd) {
	m.mu.Lock()
	m.currentCmd = cmd
	m.mu.Unlock()
}

func (m *Manager) addTemps(paths ...string) {
	m.mu.Lock()
	m.currentTemps = append(m.currentTemps, paths...)
	m.mu.Unlock()
}

func (m *Manager) endJob() {
	m.mu.Lock()
	m.currentJobID = 0
	m.currentCmd = nil
	m.currentTemps = nil
	m.jobCancel = nil
	m.mu.Unlock()
}

func (m *Manager) cleanupTemps() {
	m.mu.Lock()
	temps := append([]string{}, m.currentTemps...)
	m.mu.Unlock()
	for _, p := range temps {
		if p != "" {
			os.Remove(p)
		}
	}
}

func (m *Manager) cmdStillCurrent(cmd *exec.Cmd) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cmd != nil && m.currentCmd == cmd
}

func (m *Manager) isShuttingDown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shuttingDown
}
