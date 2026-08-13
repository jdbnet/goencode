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

const maxWorkers = 16

type activeJob struct {
	id     int
	cmd    *exec.Cmd
	temps  []string
	cancel context.CancelFunc
}

type Manager struct {
	FFmpegPath  string
	TempDir     string
	Workers     int
	TriggerChan chan struct{}
	StopChan    chan struct{}
	Broadcast   func(string, interface{})
	WebhookURL  string
	encoder     *encoder.FFmpegManager

	mu           sync.Mutex
	active       map[int]*activeJob
	shuttingDown bool
	doneChan     chan struct{}
	stopOnce     sync.Once
}

func clampWorkers(n int) int {
	if n < 1 {
		return 1
	}
	if n > maxWorkers {
		return maxWorkers
	}
	return n
}

func NewManager(ffmpegPath, tempDir, webhookURL string, workers int, broadcast func(string, interface{})) *Manager {
	return &Manager{
		FFmpegPath:  ffmpegPath,
		TempDir:     tempDir,
		Workers:     clampWorkers(workers),
		TriggerChan: make(chan struct{}, 1),
		StopChan:    make(chan struct{}),
		Broadcast:   broadcast,
		WebhookURL:  webhookURL,
		encoder:     encoder.NewManager(ffmpegPath),
		active:      make(map[int]*activeJob),
		doneChan:    make(chan struct{}),
	}
}

func (m *Manager) Start() {
	if err := db.MarkProcessingAsFailed(); err != nil {
		log.Printf("Failed to mark interrupted jobs: %v", err)
	}

	log.Printf("Queue started with %d encode worker(s)", m.Workers)

	var wg sync.WaitGroup
	for i := 0; i < m.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.workerLoop()
		}()
	}
	go func() {
		wg.Wait()
		close(m.doneChan)
	}()
	m.Trigger()
}

func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		m.mu.Lock()
		m.shuttingDown = true
		jobs := make([]*activeJob, 0, len(m.active))
		for _, j := range m.active {
			jobs = append(jobs, j)
		}
		m.mu.Unlock()

		for _, j := range jobs {
			if j.cancel != nil {
				j.cancel()
			}
			interruptCmd(j.cmd)
			go escalateKill(j.cmd, m.cmdStillCurrent)
		}

		close(m.StopChan)

		select {
		case <-m.doneChan:
		case <-time.After(15 * time.Second):
			log.Printf("Queue workers did not stop in time")
			m.mu.Lock()
			for _, j := range m.active {
				killCmd(j.cmd)
			}
			m.mu.Unlock()
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
	j, ok := m.active[id]
	m.mu.Unlock()
	if ok {
		if j.cancel != nil {
			j.cancel()
		}
		interruptCmd(j.cmd)
		go escalateKill(j.cmd, m.cmdStillCurrent)
		log.Printf("Cancelling in-progress job %d", id)
		return nil
	}
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
	m.active[id] = &activeJob{id: id, cancel: cancel}
	return ctx
}

func (m *Manager) setCurrentCmd(id int, cmd *exec.Cmd) {
	m.mu.Lock()
	if j, ok := m.active[id]; ok {
		j.cmd = cmd
	}
	m.mu.Unlock()
}

func (m *Manager) addTemps(id int, paths ...string) {
	m.mu.Lock()
	if j, ok := m.active[id]; ok {
		j.temps = append(j.temps, paths...)
	}
	m.mu.Unlock()
}

func (m *Manager) endJob(id int) {
	m.mu.Lock()
	delete(m.active, id)
	m.mu.Unlock()
}

func (m *Manager) cleanupTemps(id int) {
	m.mu.Lock()
	var temps []string
	if j, ok := m.active[id]; ok {
		temps = append([]string{}, j.temps...)
	}
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
	for _, j := range m.active {
		if j.cmd == cmd {
			return true
		}
	}
	return false
}

func (m *Manager) isShuttingDown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shuttingDown
}
