package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"goencode/internal/db"
)

func (s *Server) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	jobs, err := db.GetPendingJobs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
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
