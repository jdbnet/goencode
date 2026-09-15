package downloader

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"syscall"
	"time"

	"goencode/internal/config"
	"goencode/internal/db"
	"goencode/internal/queue"
)

type activeDownload struct {
	id     int
	cancel context.CancelFunc
	cmdPID int
}

type Manager struct {
	client    *Client
	workers   int
	queueMgr  *queue.Manager
	broadcast func(string, interface{})

	mu           sync.Mutex
	active       map[int]*activeDownload
	shuttingDown bool
	stopChan     chan struct{}
	doneChan     chan struct{}
	stopOnce     sync.Once
	triggerChan  chan struct{}
}

func NewManager(cfg config.DownloaderConfig, qm *queue.Manager, broadcast func(string, interface{})) *Manager {
	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}
	client := NewClientWithOptions(ClientOptions{
		BinaryPath:         cfg.YTDLPPath,
		CookiesFile:        cfg.CookiesFile,
		CookiesFromBrowser: cfg.CookiesFromBrowser,
	})
	return &Manager{
		client:      client,
		workers:     workers,
		queueMgr:    qm,
		broadcast:   broadcast,
		active:      make(map[int]*activeDownload),
		stopChan:    make(chan struct{}),
		doneChan:    make(chan struct{}),
		triggerChan: make(chan struct{}, 1),
	}
}

func (m *Manager) Available() bool {
	return m.client != nil && m.client.Available()
}

func (m *Manager) Client() *Client {
	return m.client
}

func (m *Manager) Start() {
	if !m.Available() {
		log.Printf("yt-dlp not found at %q; URL downloads disabled", m.client.BinaryPath)
		return
	}
	if err := db.MarkDownloadingAsFailed(); err != nil {
		log.Printf("Failed to mark interrupted download jobs: %v", err)
	}
	log.Printf("Download manager started with %d worker(s)", m.workers)

	var wg sync.WaitGroup
	for i := 0; i < m.workers; i++ {
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
		active := make([]*activeDownload, 0, len(m.active))
		for _, d := range m.active {
			active = append(active, d)
		}
		m.mu.Unlock()

		for _, d := range active {
			if d.cancel != nil {
				d.cancel()
			}
			if d.cmdPID > 0 {
				_ = syscall.Kill(-d.cmdPID, syscall.SIGTERM)
			}
		}
		close(m.stopChan)
		select {
		case <-m.doneChan:
		case <-time.After(15 * time.Second):
			log.Printf("Download workers did not stop in time")
		}
	})
}

func (m *Manager) Trigger() {
	select {
	case m.triggerChan <- struct{}{}:
	default:
	}
}

func (m *Manager) notify(event string, data interface{}) {
	if m.broadcast != nil {
		m.broadcast(event, data)
	}
}

func (m *Manager) workerLoop() {
	for {
		select {
		case <-m.stopChan:
			return
		case <-m.triggerChan:
			m.processNext()
		}
	}
}

func (m *Manager) processNext() {
	if m.isShuttingDown() {
		return
	}

	jobs, err := db.GetPendingDownloadJobs()
	if err != nil {
		log.Printf("Failed to get pending download jobs: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	job := jobs[0]
	claimed, err := db.ClaimDownloadJob(job.ID)
	if err != nil {
		log.Printf("Failed to claim download job %d: %v", job.ID, err)
		return
	}
	if !claimed {
		m.Trigger()
		return
	}

	m.runJob(job)

	if !m.isShuttingDown() {
		m.Trigger()
	}
}

func (m *Manager) runJob(job db.DownloadJob) {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.active[job.ID] = &activeDownload{id: job.ID, cancel: cancel}
	m.mu.Unlock()

	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.active, job.ID)
		m.mu.Unlock()
	}()

	m.notify("download_started", job)

	filename := job.Filename
	if filename == "" {
		filename = SanitizeFilename(job.Title)
	}

	filePath, err := m.client.Download(ctx, DownloadOptions{
		URL:       job.URL,
		FormatID:  job.FormatID,
		OutputDir: job.DestPath,
		Filename:  filename,
		ProgressFn: func(pct float64) {
			_ = db.UpdateDownloadJobProgress(job.ID, pct)
			m.notify("download_progress", map[string]interface{}{
				"id":       job.ID,
				"progress": pct,
			})
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			_ = db.CancelDownloadJob(job.ID)
			m.notify("download_cancelled", map[string]interface{}{"id": job.ID})
			return
		}
		_ = db.FailDownloadJob(job.ID, err.Error())
		m.notify("download_failed", map[string]interface{}{"id": job.ID, "error": err.Error()})
		log.Printf("Download job %d failed: %v", job.ID, err)
		return
	}

	info, statErr := os.Stat(filePath)
	var fileSize int64
	if statErr == nil {
		fileSize = info.Size()
	}

	if err := db.CompleteDownloadJob(job.ID, filePath, fileSize); err != nil {
		log.Printf("Failed to mark download job %d complete: %v", job.ID, err)
	}

	m.notify("download_completed", map[string]interface{}{
		"id":        job.ID,
		"file_path": filePath,
		"file_size": fileSize,
		"mode":      job.Mode,
	})

	if job.Mode == "encode" {
		if err := m.enqueueEncode(job, filePath, fileSize); err != nil {
			log.Printf("Download job %d completed but encode enqueue failed: %v", job.ID, err)
		}
	}
}

func (m *Manager) enqueueEncode(job db.DownloadJob, filePath string, fileSize int64) error {
	if job.WatchFolderID == nil {
		return fmt.Errorf("watch folder is required for encode mode")
	}
	folder, err := db.GetWatchFolderByID(*job.WatchFolderID)
	if err != nil {
		return fmt.Errorf("watch folder not found: %w", err)
	}

	already, err := db.IsFileAlreadyProcessedOrQueued(filePath)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	encodeJob := folder.NewJob(filePath, fileSize, 5)
	if err := db.AddJob(encodeJob); err != nil {
		return err
	}
	if m.queueMgr != nil {
		m.queueMgr.NotifySSE("job_added", nil)
		m.queueMgr.NotifySSE("queue_updated", nil)
		m.queueMgr.Trigger()
	}
	return nil
}

func (m *Manager) Cancel(id int) error {
	m.mu.Lock()
	d, ok := m.active[id]
	m.mu.Unlock()
	if ok {
		if d.cancel != nil {
			d.cancel()
		}
		if d.cmdPID > 0 {
			_ = syscall.Kill(-d.cmdPID, syscall.SIGTERM)
		}
		return nil
	}
	return db.CancelDownloadJob(id)
}

func (m *Manager) isShuttingDown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shuttingDown
}
