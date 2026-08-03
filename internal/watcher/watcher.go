package watcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"goencode/internal/db"
	"goencode/internal/encoder"
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

		filepath.Walk(f.FolderPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Printf("Error accessing path %s during scan: %v", path, err)
				return nil
			}
			if info.IsDir() {
				if err := m.watcher.Add(path); err != nil {
					log.Printf("Failed to watch %s: %v", path, err)
				}
			} else {
				go m.handleEvent(path)
			}
			return nil
		})
	}
}

// ScanFolder immediately scans all files in a watch folder, skipping debounce and stability waits.
func (m *Manager) ScanFolder(id int) (int, error) {
	folders, err := db.GetWatchFolders()
	if err != nil {
		return 0, err
	}

	var folder *db.WatchFolder
	for _, f := range folders {
		if f.ID == id {
			if !f.Enabled {
				return 0, fmt.Errorf("folder is disabled")
			}
			folder = &f
			break
		}
	}
	if folder == nil {
		return 0, fmt.Errorf("folder not found")
	}

	count := 0
	err = filepath.Walk(folder.FolderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error accessing path %s during scan: %v", path, err)
			return nil
		}
		if !info.IsDir() {
			count++
			go m.processFileImmediate(path)
		}
		return nil
	})
	if err != nil {
		return count, err
	}

	log.Printf("Manual scan started for %s (%d files)", folder.FolderPath, count)
	return count, nil
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
			go m.processFile(path, true)
		}
	}
}

func (m *Manager) handleEvent(filePath string) {
	m.timersMu.Lock()
	defer m.timersMu.Unlock()

	if t, exists := m.timers[filePath]; exists {
		t.Stop()
	}

	m.timers[filePath] = time.AfterFunc(2*time.Minute, func() {
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

func (m *Manager) processFileImmediate(filePath string) {
	m.processFile(filePath, false)
}

func (m *Manager) processFile(filePath string, waitForStable bool) {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return
	}

	if filepath.Ext(filePath) == ".tmp" {
		return
	}

	if waitForStable {
		var lastSize int64 = info.Size()
		var lastModTime time.Time = info.ModTime()
		var stableCount int

		for {
			time.Sleep(5 * time.Second)
			currentInfo, err := os.Stat(filePath)
			if err != nil {
				return
			}

			if currentInfo.Size() == lastSize && currentInfo.ModTime().Equal(lastModTime) {
				stableCount++
				if stableCount >= 3 {
					info = currentInfo
					break
				}
			} else {
				stableCount = 0
				lastSize = currentInfo.Size()
				lastModTime = currentInfo.ModTime()
			}
		}
	}

	alreadyInQueue, err := db.IsFileAlreadyProcessedOrQueued(filePath)
	if err != nil {
		log.Printf("Error checking DB for %s: %v", filePath, err)
		return
	}
	if alreadyInQueue {
		return
	}

	match, found := findWatchFolder(filePath)
	if !found {
		return
	}

	if skip, reason := checkSkip(filePath, match); skip {
		job := db.Job{
			FilePath:         filePath,
			MediaType:        match.MediaType,
			TargetResolution: match.TargetResolution,
			FFmpegFlags:      match.CustomFFmpegFlags,
			OriginalSize:     info.Size(),
			ErrorMessage:     reason,
		}
		if err := db.AddJobReport(job, "skipped", info.Size(), 0, 0); err != nil {
			log.Printf("Failed to record skip for %s: %v", filePath, err)
			return
		}
		log.Printf("Skipped %s: %s", filePath, reason)
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

func checkSkip(filePath string, folder db.WatchFolder) (bool, string) {
	if folder.MediaType == "video" {
		return encoder.CheckVideoSkip(filePath, folder.TargetResolution)
	}
	return encoder.CheckAudioSkip(filePath)
}

func findWatchFolder(filePath string) (db.WatchFolder, bool) {
	folders, err := db.GetWatchFolders()
	if err != nil {
		return db.WatchFolder{}, false
	}

	cleanPath := filepath.Clean(filePath)
	for _, f := range folders {
		if !f.Enabled {
			continue
		}
		folderPath := filepath.Clean(strings.TrimSpace(f.FolderPath))

		var isMatch bool
		if cleanPath == folderPath {
			isMatch = true
		} else if folderPath == string(os.PathSeparator) {
			isMatch = strings.HasPrefix(cleanPath, folderPath)
		} else {
			isMatch = strings.HasPrefix(cleanPath, folderPath+string(os.PathSeparator))
		}

		if isMatch {
			return f, true
		}
	}
	return db.WatchFolder{}, false
}
