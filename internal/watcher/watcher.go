package watcher

import (
	"errors"
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

const (
	debounceDuration  = 2 * time.Minute
	stablePoll        = 5 * time.Second
	stableNeeded      = 3
	maxStableWait     = 30 * time.Minute
	periodicScanEvery = time.Minute
	dirScanDebounce   = 2 * time.Second
	eventBuffer       = 4096
)

var ignoredExts = map[string]struct{}{
	".tmp": {}, ".temp": {}, ".part": {}, ".partial": {},
	".crdownload": {}, ".download": {}, ".filepart": {},
	".!qb": {}, ".!ut": {}, ".bak": {}, ".swp": {},
}

type Manager struct {
	watcher      *fsnotify.Watcher
	queueManager *queue.Manager
	timers       map[string]*time.Timer
	timersMu     sync.Mutex
	dirTimers    map[string]*time.Timer
	dirTimersMu  sync.Mutex
	scanMu       sync.Mutex
	processChan  chan string
	stopChan     chan struct{}
}

func NewManager(qm *queue.Manager) (*Manager, error) {
	w, err := fsnotify.NewBufferedWatcher(eventBuffer)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		watcher:      w,
		queueManager: qm,
		timers:       make(map[string]*time.Timer),
		dirTimers:    make(map[string]*time.Timer),
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
	go m.periodicScanLoop()
}

func (m *Manager) Stop() {
	close(m.stopChan)
	m.watcher.Close()
}

func (m *Manager) Reload() {
	m.scanFolders(true)
}

func (m *Manager) periodicScanLoop() {
	ticker := time.NewTicker(periodicScanEvery)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.scanFolders(false)
		}
	}
}

func (m *Manager) scanFolders(removeStale bool) {
	if m.stopped() {
		return
	}

	m.scanMu.Lock()
	defer m.scanMu.Unlock()

	folders, err := db.GetWatchFolders()
	if err != nil {
		log.Printf("Watcher failed to get folders: %v", err)
		return
	}

	var enabledRoots []string
	for _, f := range folders {
		if !f.Enabled {
			continue
		}
		root := filepath.Clean(strings.TrimSpace(f.FolderPath))
		if err := os.MkdirAll(root, 0755); err != nil {
			log.Printf("Failed to create watch folder %s: %v", root, err)
			continue
		}
		if removeStale {
			log.Printf("Watching and scanning %s", root)
		}
		enabledRoots = append(enabledRoots, root)
		m.walkAndWatch(root, false)
	}

	if !removeStale {
		return
	}

	for _, path := range m.watcher.WatchList() {
		if !underAnyRoot(path, enabledRoots) {
			m.watcher.Remove(path)
		}
	}
}

func (m *Manager) walkAndWatch(root string, resetDebounce bool) {
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("Error accessing path %s during scan: %v", path, err)
			return nil
		}
		if d.IsDir() {
			if addErr := m.watcher.Add(path); addErr != nil {
				log.Printf("Failed to watch %s: %v", path, addErr)
			}
			return nil
		}
		if resetDebounce {
			m.handleEvent(path)
		} else {
			m.handleEventIfIdle(path)
		}
		return nil
	})
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
	err = filepath.WalkDir(folder.FolderPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("Error accessing path %s during scan: %v", path, err)
			return nil
		}
		if !d.IsDir() {
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
			m.dispatchEvent(event)
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				go m.scanFolders(false)
			}
		}
	}
}

func (m *Manager) dispatchEvent(event fsnotify.Event) {
	if event.Has(fsnotify.Remove) {
		return
	}

	interesting := event.Has(fsnotify.Create) || event.Has(fsnotify.Write) ||
		event.Has(fsnotify.Rename) || event.Has(fsnotify.Chmod)
	if !interesting {
		return
	}

	info, err := os.Stat(event.Name)
	if err != nil {
		if event.Has(fsnotify.Rename) {
			m.enqueueDirScan(filepath.Dir(event.Name))
		}
		return
	}

	if info.IsDir() {
		if event.Has(fsnotify.Create) {
			go m.walkAndWatch(event.Name, true)
		} else {
			m.enqueueDirScan(event.Name)
		}
		return
	}

	if event.Has(fsnotify.Chmod) && !event.Has(fsnotify.Write) &&
		!event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
		m.handleEventIfIdle(event.Name)
		return
	}

	m.handleEvent(event.Name)
}

func (m *Manager) enqueueDirScan(dir string) {
	if dir == "" || dir == "." {
		return
	}
	dir = filepath.Clean(dir)

	m.dirTimersMu.Lock()
	defer m.dirTimersMu.Unlock()

	if t, exists := m.dirTimers[dir]; exists {
		t.Stop()
	}

	m.dirTimers[dir] = time.AfterFunc(dirScanDebounce, func() {
		m.dirTimersMu.Lock()
		delete(m.dirTimers, dir)
		m.dirTimersMu.Unlock()

		if m.stopped() {
			return
		}
		m.walkAndWatch(dir, true)
	})
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
	m.scheduleProcess(filePath, true)
}

func (m *Manager) handleEventIfIdle(filePath string) {
	if shouldIgnoreFile(filePath) {
		return
	}
	filePath = filepath.Clean(filePath)

	m.timersMu.Lock()
	_, pending := m.timers[filePath]
	m.timersMu.Unlock()
	if pending {
		return
	}

	already, err := db.IsFileAlreadyProcessedOrQueued(filePath)
	if err != nil {
		log.Printf("Error checking DB for %s: %v", filePath, err)
		return
	}
	if already {
		return
	}

	m.scheduleProcess(filePath, false)
}

func (m *Manager) scheduleProcess(filePath string, reset bool) {
	if shouldIgnoreFile(filePath) {
		return
	}
	filePath = filepath.Clean(filePath)

	m.timersMu.Lock()
	defer m.timersMu.Unlock()

	if t, exists := m.timers[filePath]; exists {
		if !reset {
			return
		}
		t.Stop()
	}

	m.timers[filePath] = time.AfterFunc(debounceDuration, func() {
		m.timersMu.Lock()
		delete(m.timers, filePath)
		m.timersMu.Unlock()

		if m.stopped() {
			return
		}

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

	if shouldIgnoreFile(filePath) {
		return
	}

	if waitForStable {
		var lastSize int64 = info.Size()
		var stableCount int
		deadline := time.Now().Add(maxStableWait)

		for {
			if time.Now().After(deadline) {
				log.Printf("File %s did not stabilize in time, will retry later", filePath)
				return
			}

			time.Sleep(stablePoll)
			currentInfo, err := os.Stat(filePath)
			if err != nil {
				return
			}

			if currentInfo.Size() == lastSize {
				stableCount++
				if stableCount >= stableNeeded {
					info = currentInfo
					break
				}
			} else {
				stableCount = 0
				lastSize = currentInfo.Size()
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
		if pathUnderFolder(cleanPath, folderPath) {
			return f, true
		}
	}
	return db.WatchFolder{}, false
}

func underAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if pathUnderFolder(path, root) {
			return true
		}
	}
	return false
}

func pathUnderFolder(path, folder string) bool {
	path = filepath.Clean(path)
	folder = filepath.Clean(folder)
	if path == folder {
		return true
	}
	if folder == string(os.PathSeparator) {
		return strings.HasPrefix(path, folder)
	}
	return strings.HasPrefix(path, folder+string(os.PathSeparator))
}

func shouldIgnoreFile(filePath string) bool {
	base := filepath.Base(filePath)
	if base == "" || base == "." || base == ".." || strings.HasPrefix(base, ".") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	_, ignored := ignoredExts[ext]
	return ignored
}

func (m *Manager) stopped() bool {
	select {
	case <-m.stopChan:
		return true
	default:
		return false
	}
}
