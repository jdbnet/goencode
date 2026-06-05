package watcher

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"goencode/internal/db"
	"goencode/internal/queue"
)

type Manager struct {
	watcher      *fsnotify.Watcher
	queueManager *queue.Manager
	timers       map[string]*time.Timer
	timersMu     sync.Mutex
	processChan  chan string
	stopChan     chan struct{}
}

func NewManager(qm *queue.Manager) (*Manager, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	m := &Manager{
		watcher:      w,
		queueManager: qm,
		timers:       make(map[string]*time.Timer),
		processChan:  make(chan string, 10000),
		stopChan:     make(chan struct{}),
	}
	
	for i := 0; i < 3; i++ {
		go m.processWorker()
	}
	
	return m, nil
}

func (m *Manager) Start() {
	m.Reload()
	go m.watchLoop()
}

func (m *Manager) Stop() {
	close(m.stopChan)
	m.watcher.Close()
}

func (m *Manager) Reload() {
	// Remove all existing watches
	for _, path := range m.watcher.WatchList() {
		m.watcher.Remove(path)
	}

	folders, err := db.GetWatchFolders()
	if err != nil {
		log.Printf("Watcher failed to get folders: %v", err)
		return
	}

	for _, f := range folders {
		if !f.Enabled {
			continue
		}
		if err := os.MkdirAll(f.FolderPath, 0755); err != nil {
			log.Printf("Failed to create watch folder %s: %v", f.FolderPath, err)
			continue
		}
		log.Printf("Watching and scanning %s", f.FolderPath)
		
		// Walk the directory to add all subdirectories to watcher and scan existing files
		filepath.Walk(f.FolderPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if err := m.watcher.Add(path); err != nil {
					log.Printf("Failed to watch %s: %v", path, err)
				}
			} else {
				// Process existing file asynchronously
				go m.handleEvent(path)
			}
			return nil
		})
	}
}

func (m *Manager) watchLoop() {
	for {
		select {
		case <-m.stopChan:
			return
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Check if the created event is a directory
				if event.Has(fsnotify.Create) {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						filepath.Walk(event.Name, func(path string, info os.FileInfo, err error) error {
							if err != nil {
								return nil
							}
							if info.IsDir() {
								m.watcher.Add(path)
							} else {
								go m.handleEvent(path)
							}
							return nil
						})
					}
				}
				m.handleEvent(event.Name)
			}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func (m *Manager) processWorker() {
	for {
		select {
		case <-m.stopChan:
			return
		case path := <-m.processChan:
			m.processFile(path)
		}
	}
}

func (m *Manager) handleEvent(filePath string) {
	m.timersMu.Lock()
	defer m.timersMu.Unlock()

	if t, exists := m.timers[filePath]; exists {
		t.Stop()
	}

	m.timers[filePath] = time.AfterFunc(5*time.Second, func() {
		m.timersMu.Lock()
		delete(m.timers, filePath)
		m.timersMu.Unlock()
		
		select {
		case m.processChan <- filePath:
		default:
			log.Printf("Process queue full, dropping %s", filePath)
		}
	})
}

func (m *Manager) processFile(filePath string) {

	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return // File removed or is a directory
	}

	// Make sure it's not a temp file
	if filepath.Ext(filePath) == ".tmp" {
		return
	}

	alreadyInQueue, err := db.IsFileAlreadyProcessedOrQueued(filePath)
	if err != nil {
		log.Printf("Error checking DB for %s: %v", filePath, err)
		return
	}
	if alreadyInQueue {
		return
	}

	// Find which watch folder it belongs to
	folders, err := db.GetWatchFolders()
	if err != nil {
		return
	}

	var match db.WatchFolder
	found := false
	for _, f := range folders {
		if !f.Enabled {
			continue
		}
		// Check if filePath is inside f.FolderPath
		cleanPath := filepath.Clean(filePath)
		folderPath := filepath.Clean(f.FolderPath)
		if strings.HasPrefix(cleanPath, folderPath+string(os.PathSeparator)) || cleanPath == folderPath || filepath.Dir(cleanPath) == folderPath {
			match = f
			found = true
			break
		}
	}

	if !found {
		return
	}

	err = db.AddJob(filePath, match.MediaType, 0, match.TargetResolution, match.CustomFFmpegFlags, info.Size())
	if err != nil {
		log.Printf("Failed to add job for %s: %v", filePath, err)
		return
	}

	log.Printf("Added job for %s", filePath)
	m.queueManager.NotifySSE("job_added", nil)
	m.queueManager.Trigger()
}
