package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"goencode/internal/db"
)

func (s *Server) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	pageStr := r.URL.Query().Get("page")

	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	offset := (page - 1) * limit

	jobs, total, err := db.GetJobsPaginated(limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	response := map[string]interface{}{
		"jobs":       jobs,
		"total":      total,
		"page":       page,
		"totalPages": totalPages,
		"limit":      limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleBumpJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/jobs/bump/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if err := db.BumpJobPriority(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.qm.NotifySSE("queue_updated", nil)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/jobs/cancel/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if err := db.DeleteJob(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.qm.NotifySSE("queue_updated", nil)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetWatchFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := db.GetWatchFolders()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folders)
}

func (s *Server) handleAddWatchFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var f db.WatchFolder
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.FolderPath = strings.TrimSpace(f.FolderPath)
	f.OutputDir = strings.TrimSpace(f.OutputDir)
	f.Enabled = true
	if err := db.AddWatchFolder(f); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.wm.Reload()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUpdateWatchFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/folders/update/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	existing, err := db.GetWatchFolderByID(id)
	if err != nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}
	var f db.WatchFolder
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.ID = id
	f.FolderPath = strings.TrimSpace(f.FolderPath)
	f.OutputDir = strings.TrimSpace(f.OutputDir)
	if f.FolderPath == "" {
		http.Error(w, "folder_path is required", http.StatusBadRequest)
		return
	}

	pathChanged := f.FolderPath != existing.FolderPath
	if pathChanged {
		s.wm.DropPendingForFolder(existing.FolderPath)
	}
	if err := db.UpdateWatchFolder(f); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if pathChanged {
		s.wm.ReplaceFolderWatch(existing.FolderPath, f.FolderPath)
		n, err := db.DeleteJobsUnderPath(existing.FolderPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if n > 0 {
			log.Printf("Removed %d queued jobs after watch folder path change %s -> %s", n, existing.FolderPath, f.FolderPath)
		}
		s.qm.NotifySSE("queue_updated", nil)
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeleteWatchFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/folders/delete/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	folder, err := db.GetWatchFolderByID(id)
	if err != nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}
	s.wm.DropPendingForFolder(folder.FolderPath)
	if err := db.DeleteWatchFolder(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.wm.Reload()
	n, err := db.DeleteJobsUnderPath(folder.FolderPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n > 0 {
		log.Printf("Removed %d queued jobs for deleted watch folder %s", n, folder.FolderPath)
	}
	s.qm.NotifySSE("queue_updated", nil)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSetWatchFolderEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/folders/enabled/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Enabled == nil {
		http.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}
	enabled := *body.Enabled

	folder, err := db.GetWatchFolderByID(id)
	if err != nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	if err := db.SetWatchFolderEnabled(id, enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !enabled {
		s.wm.DropPendingForFolder(folder.FolderPath)
	}

	s.wm.Reload()

	if !enabled {
		n, err := db.DeleteJobsUnderPath(folder.FolderPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("Disabled watch folder %s (removed %d queued jobs)", folder.FolderPath, n)
		s.qm.NotifySSE("queue_updated", nil)
	} else {
		log.Printf("Enabled watch folder %s", folder.FolderPath)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleScanWatchFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/folders/scan/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	count, err := s.wm.ScanFolder(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"files_scanned": count,
	})
}

func (s *Server) handleRequeueJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/jobs/requeue/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	report, err := db.GetJobReportByID(id)
	if err != nil {
		http.Error(w, "Job report not found", http.StatusNotFound)
		return
	}

	if report.Status != "failed" {
		http.Error(w, "Can only requeue failed jobs", http.StatusBadRequest)
		return
	}

	err = db.AddJob(db.Job{
		FilePath:         report.FilePath,
		MediaType:        report.MediaType,
		Priority:         5,
		OriginalSize:     report.OriginalSize,
		TargetResolution: report.TargetResolution,
		FFmpegFlags:      report.FFmpegFlags,
		EncodeSettings:   report.EncodeSettings,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the failed report so it doesn't show up in history as failed anymore
	_ = db.DeleteJobReport(id)

	s.qm.NotifySSE("queue_updated", nil)
	s.qm.NotifySSE("job_added", nil)
	s.qm.Trigger()
	w.WriteHeader(http.StatusOK)
}
