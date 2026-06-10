package web

import (
	"encoding/json"
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
	f.Enabled = true
	if err := db.AddWatchFolder(f); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.wm.Reload()
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
	if err := db.DeleteWatchFolder(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.wm.Reload()
	w.WriteHeader(http.StatusOK)
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
	
	err = db.AddJob(report.FilePath, report.MediaType, 5, report.TargetResolution, report.FFmpegFlags, report.OriginalSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Delete the failed report so it doesn't show up in history as failed anymore
	_ = db.DeleteJobReport(id)
	
	s.qm.NotifySSE("queue_updated", nil)
	s.qm.NotifySSE("job_added", nil)
	w.WriteHeader(http.StatusOK)
}
