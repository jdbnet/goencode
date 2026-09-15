package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"goencode/internal/db"
	rootweb "goencode/web"
)

type FSEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type FSListResponse struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent"`
	Entries []FSEntry `json:"entries"`
}

func cleanBrowsePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if wd, err := os.Getwd(); err == nil {
			return filepath.Clean(wd), nil
		}
		return "/", nil
	}
	clean := filepath.Clean(raw)
	if clean == "." {
		if wd, err := os.Getwd(); err == nil {
			return filepath.Clean(wd), nil
		}
	}
	return clean, nil
}

func parentPath(path string) string {
	parent := filepath.Dir(path)
	if parent == path {
		if path == string(filepath.Separator) {
			return path
		}
		return string(filepath.Separator)
	}
	return parent
}

func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := cleanBrowsePath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("path not accessible: %v", err), http.StatusBadRequest)
		return
	}
	if !info.IsDir() {
		path = filepath.Dir(path)
		info, err = os.Stat(path)
		if err != nil {
			http.Error(w, fmt.Sprintf("path not accessible: %v", err), http.StatusBadRequest)
			return
		}
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("read directory: %v", err), http.StatusInternalServerError)
		return
	}

	resp := FSListResponse{
		Path:   path,
		Parent: parentPath(path),
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		entryPath := filepath.Join(path, name)
		entry := FSEntry{
			Name:  name,
			Path:  entryPath,
			IsDir: e.IsDir(),
		}
		if !e.IsDir() {
			if fi, err := e.Info(); err == nil {
				entry.Size = fi.Size()
			}
		}
		resp.Entries = append(resp.Entries, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleDownloadProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dm == nil || !s.dm.Available() {
		http.Error(w, "yt-dlp is not available on this server", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := s.dm.Client().Probe(r.Context(), body.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleDownloadStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dm == nil || !s.dm.Available() {
		http.Error(w, "yt-dlp is not available on this server", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		URL           string `json:"url"`
		Title         string `json:"title"`
		FormatID      string `json:"format_id"`
		Filename      string `json:"filename"`
		DestPath      string `json:"dest_path"`
		Mode          string `json:"mode"`
		WatchFolderID *int   `json:"watch_folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body.URL = strings.TrimSpace(body.URL)
	body.FormatID = strings.TrimSpace(body.FormatID)
	body.DestPath = strings.TrimSpace(body.DestPath)
	body.Mode = strings.TrimSpace(body.Mode)
	body.Filename = strings.TrimSpace(body.Filename)

	if body.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	if body.FormatID == "" {
		body.FormatID = "bestvideo+bestaudio/best"
	}
	if body.Mode != "encode" && body.Mode != "download_only" {
		http.Error(w, "mode must be encode or download_only", http.StatusBadRequest)
		return
	}

	destPath := body.DestPath
	if body.Mode == "encode" {
		if body.WatchFolderID == nil || *body.WatchFolderID <= 0 {
			http.Error(w, "watch_folder_id is required for encode mode", http.StatusBadRequest)
			return
		}
		folder, err := db.GetWatchFolderByID(*body.WatchFolderID)
		if err != nil {
			http.Error(w, "watch folder not found", http.StatusBadRequest)
			return
		}
		destPath = folder.FolderPath
	} else {
		if destPath == "" {
			http.Error(w, "dest_path is required for download_only mode", http.StatusBadRequest)
			return
		}
		destPath = filepath.Clean(destPath)
		if err := os.MkdirAll(destPath, 0755); err != nil {
			http.Error(w, fmt.Sprintf("create destination: %v", err), http.StatusBadRequest)
			return
		}
	}

	job := db.DownloadJob{
		URL:           body.URL,
		Title:         body.Title,
		Filename:      body.Filename,
		DestPath:      destPath,
		FormatID:      body.FormatID,
		Mode:          body.Mode,
		WatchFolderID: body.WatchFolderID,
	}
	id, err := db.AddDownloadJob(job)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.dm.Trigger()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": id,
	})
}

func (s *Server) handleGetDownloadJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}

	jobs, err := db.GetDownloadJobs(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (s *Server) handleCancelDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/download/cancel/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if s.dm == nil {
		http.Error(w, "download manager not available", http.StatusServiceUnavailable)
		return
	}
	if err := s.dm.Cancel(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.sse.Broadcast("download_cancelled", map[string]interface{}{"id": id})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/download/file/"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	job, err := db.GetDownloadJobByID(id)
	if err != nil {
		http.Error(w, "Download job not found", http.StatusNotFound)
		return
	}
	if job.Status != "completed" || job.FilePath == "" {
		http.Error(w, "File is not ready", http.StatusNotFound)
		return
	}

	f, err := os.Open(job.FilePath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "File not accessible", http.StatusInternalServerError)
		return
	}

	filename := filepath.Base(job.FilePath)
	if job.Filename != "" {
		ext := filepath.Ext(job.FilePath)
		filename = job.Filename + ext
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	io.Copy(w, f)
}

func (s *Server) handleDownloadPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		AuthEnabled       bool
		YTDLPAvailable    bool
		YTDLPCookiesReady bool
		Version           string
	}{
		AuthEnabled:       s.cfg.Auth.Username != "",
		YTDLPAvailable:    s.dm != nil && s.dm.Available(),
		YTDLPCookiesReady: s.dm != nil && s.dm.Client().CookiesConfigured(),
		Version:           s.version,
	}

	tmpl, err := template.New("layout").Funcs(template.FuncMap{
		"formatBytes": formatBytes,
	}).ParseFS(rootweb.FS, "templates/layout.html", "templates/download.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
