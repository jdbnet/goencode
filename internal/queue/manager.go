package queue

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"goencode/internal/db"
	"goencode/internal/encoder"
	"goencode/internal/notify"
)

const maxWorkers = 16

type activeJob struct {
	id     int
	cmd    *exec.Cmd
	temps  []string
	cancel context.CancelFunc
}

type Manager struct {
	FFmpegPath   string
	TempDir      string
	Workers      int
	Threads      int
	TriggerChan  chan struct{}
	StopChan     chan struct{}
	Broadcast    func(string, interface{})
	Notifier     notify.Notifier
	MinFreeBytes int64
	encoder      *encoder.FFmpegManager

	mu           sync.Mutex
	active       map[int]*activeJob
	shuttingDown bool
	doneChan     chan struct{}
	stopOnce     sync.Once
	loc          *time.Location
	paused       bool
	windowStart  string
	windowEnd    string
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

func NewManager(ffmpegPath, tempDir string, workers int, loc *time.Location, broadcast func(string, interface{})) *Manager {
	if loc == nil {
		loc = time.Local
	}
	return &Manager{
		FFmpegPath:   ffmpegPath,
		TempDir:      tempDir,
		Workers:      clampWorkers(workers),
		TriggerChan:  make(chan struct{}, 1),
		StopChan:     make(chan struct{}),
		Broadcast:    broadcast,
		MinFreeBytes: defaultMinFreeBytes,
		encoder:      encoder.NewManager(ffmpegPath),
		active:       make(map[int]*activeJob),
		doneChan:     make(chan struct{}),
		loc:          loc,
	}
}

func (m *Manager) SetThreads(n int) {
	if n < 0 {
		n = 0
	}
	m.Threads = n
	if m.encoder != nil {
		m.encoder.Threads = n
	}
}

func (m *Manager) Start() {
	if err := db.MarkProcessingAsFailed(); err != nil {
		log.Printf("Failed to mark interrupted jobs: %v", err)
	}

	m.loadSchedule()
	log.Printf("Queue started with %d encode worker(s), min free %s", m.Workers, formatBytes(m.minFree()))
	if st := m.ScheduleState(); !st.Allowed {
		log.Printf("Queue idle: %s", st.Reason)
	}

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
	go m.scheduleLoop()
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

func (m *Manager) minFree() int64 {
	if m.MinFreeBytes < 0 {
		return 0
	}
	if m.MinFreeBytes == 0 {
		return defaultMinFreeBytes
	}
	return m.MinFreeBytes
}

func (m *Manager) isShuttingDown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shuttingDown
}

func (m *Manager) loadSchedule() {
	pausedVal, err := db.GetAppConfig(configQueuePaused)
	if err != nil {
		log.Printf("Failed to load pause state: %v", err)
	}
	start, err := db.GetAppConfig(configWindowStart)
	if err != nil {
		log.Printf("Failed to load encode window start: %v", err)
	}
	end, err := db.GetAppConfig(configWindowEnd)
	if err != nil {
		log.Printf("Failed to load encode window end: %v", err)
	}

	m.mu.Lock()
	m.paused = pausedVal == "1" || strings.EqualFold(pausedVal, "true")
	m.windowStart = start
	m.windowEnd = end
	m.mu.Unlock()
}

func (m *Manager) Allowed() bool {
	st := m.ScheduleState()
	return st.Allowed
}

func (m *Manager) ScheduleState() ScheduleState {
	m.mu.Lock()
	paused := m.paused
	start := m.windowStart
	end := m.windowEnd
	loc := m.loc
	m.mu.Unlock()

	now := time.Now().In(loc)
	inWindow := inEncodeWindow(now, start, end)
	st := ScheduleState{
		Paused:      paused,
		WindowStart: start,
		WindowEnd:   end,
		InWindow:    inWindow,
		Timezone:    loc.String(),
		Allowed:     !paused && inWindow,
	}
	if paused {
		st.Reason = "paused"
	} else if !inWindow {
		st.Reason = "outside_window"
	}
	return st
}

func (m *Manager) SetPaused(paused bool) error {
	val := "0"
	if paused {
		val = "1"
	}
	if err := db.SetAppConfig(configQueuePaused, val); err != nil {
		return err
	}
	m.mu.Lock()
	m.paused = paused
	m.mu.Unlock()
	if paused {
		log.Printf("Queue paused")
	} else {
		log.Printf("Queue resumed")
		m.Trigger()
	}
	m.NotifySSE("queue_schedule", m.ScheduleState())
	return nil
}

func (m *Manager) SetWindow(start, end string) error {
	start, err := normalizeClock(start)
	if err != nil {
		return err
	}
	end, err = normalizeClock(end)
	if err != nil {
		return err
	}
	if (start == "") != (end == "") {
		return fmt.Errorf("set both window start and end, or clear both")
	}
	if err := db.SetAppConfig(configWindowStart, start); err != nil {
		return err
	}
	if err := db.SetAppConfig(configWindowEnd, end); err != nil {
		return err
	}
	m.mu.Lock()
	m.windowStart = start
	m.windowEnd = end
	m.mu.Unlock()
	if start == "" {
		log.Printf("Encode window cleared")
	} else {
		log.Printf("Encode window set to %s-%s (%s)", start, end, m.loc)
	}
	m.Trigger()
	m.NotifySSE("queue_schedule", m.ScheduleState())
	return nil
}

func (m *Manager) scheduleLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	prev := m.Allowed()
	for {
		select {
		case <-m.StopChan:
			return
		case <-ticker.C:
			now := m.Allowed()
			if now && !prev {
				log.Printf("Encode window open, starting queue")
				m.Trigger()
				m.NotifySSE("queue_schedule", m.ScheduleState())
			} else if !now && prev {
				log.Printf("Encode window closed, not starting new jobs")
				m.NotifySSE("queue_schedule", m.ScheduleState())
			}
			prev = now
		}
	}
}
