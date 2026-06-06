package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"

	"goencode/internal/config"
	"goencode/internal/db"
	"goencode/internal/logger"
	"goencode/internal/queue"
	"goencode/internal/watcher"
	rootweb "goencode/web"
)

type Server struct {
	cfg *config.Config
	qm  *queue.Manager
	wm  *watcher.Manager
	sse *SSEServer
	mux *http.ServeMux
	sessionToken string
}

func NewServer(cfg *config.Config, qm *queue.Manager, wm *watcher.Manager, sse *SSEServer) *Server {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	s := &Server{
		cfg:          cfg,
		qm:           qm,
		wm:           wm,
		sse:          sse,
		mux:          http.NewServeMux(),
		sessionToken: token,
	}
	s.routes()
	return s
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.ListenAddr, s.cfg.Server.Port)
	
	var handler http.Handler = s.mux
	if s.cfg.Auth.Username != "" {
		handler = s.authMiddleware(s.mux)
	}
	
	return http.ListenAndServe(addr, handler)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		importStrings := true // to ensure "strings" is imported if not already, wait, I need to check if strings is imported.
		_ = importStrings
		
		// We'll use a simple slice of prefixes to bypass auth, but since there's only one, 
		// let's just use string slicing or import "strings"
		
		// To be safe without adding imports manually if I don't know the exact list:
		if s.cfg.Auth.Username == "" || r.URL.Path == "/login" || r.URL.Path == "/api/status" || (len(r.URL.Path) >= 8 && r.URL.Path[:8] == "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie("goencode_session")
		if err != nil || cookie.Value != s.sessionToken {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		tmpl, err := template.ParseFS(rootweb.FS, "templates/login.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
		return
	}

	if r.Method == "POST" {
		user := r.FormValue("username")
		pass := r.FormValue("password")

		if user == s.cfg.Auth.Username && pass == s.cfg.Auth.Password {
			http.SetCookie(w, &http.Cookie{
				Name:     "goencode_session",
				Value:    s.sessionToken,
				Path:     "/",
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		tmpl, _ := template.ParseFS(rootweb.FS, "templates/login.html")
		tmpl.Execute(w, struct{ Error string }{"Invalid credentials"})
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "goencode_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)

	s.mux.HandleFunc("/api/sse", s.sse.HandleSSE)
	
	s.mux.HandleFunc("/api/queue", s.handleGetQueue)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/jobs/bump/", s.handleBumpJob)
	s.mux.HandleFunc("/api/jobs/cancel/", s.handleCancelJob)
	
	s.mux.HandleFunc("/api/folders", s.handleGetWatchFolders)
	s.mux.HandleFunc("/api/folders/add", s.handleAddWatchFolder)
	s.mux.HandleFunc("/api/folders/delete/", s.handleDeleteWatchFolder)
	s.mux.HandleFunc("/api/logs", s.handleGetLogs)

	// Static files
	s.mux.Handle("/static/", http.FileServer(http.FS(rootweb.FS)))

	// Pages
	s.mux.HandleFunc("/", s.handleDashboard)
	s.mux.HandleFunc("/folders", s.handlePage("folders.html"))
	s.mux.HandleFunc("/history", s.handleHistory)
	s.mux.HandleFunc("/logs", s.handlePage("logs.html"))
	s.mux.HandleFunc("/settings", s.handlePage("settings.html"))
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	logs := logger.GetRecentLogs()
	json.NewEncoder(w).Encode(logs)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats, err := db.GetDashboardStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jobs, err := db.GetPendingJobs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var processingJob *db.Job
	for i := range jobs {
		if jobs[i].Status == "processing" {
			processingJob = &jobs[i]
			break
		}
	}

	response := map[string]interface{}{
		"stats": map[string]interface{}{
			"files_encoded":         stats.FilesEncoded,
			"saved_space_bytes":     stats.TotalSavedSpace,
			"saved_space_formatted": formatBytes(stats.TotalSavedSpace),
			"queue_length":          stats.QueueLength,
		},
		"current_job": processingJob,
	}

	json.NewEncoder(w).Encode(response)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit && bytes > -unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	n := bytes
	if n < 0 {
		n = -n
	}
	for n >= unit*unit && exp < len("KMGTPE")-1 {
		div *= unit
		exp++
		n /= unit
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	stats, _ := db.GetDashboardStats()
	
	// Create formatted stats
	data := struct {
		AuthEnabled     bool
		FilesEncoded    int
		QueueLength     int
		SavedSpace      string
	}{
		AuthEnabled:  s.cfg.Auth.Username != "",
		FilesEncoded: stats.FilesEncoded,
		QueueLength:  stats.QueueLength,
		SavedSpace:   formatBytes(stats.TotalSavedSpace),
	}

	tmpl, err := template.New("layout").Funcs(template.FuncMap{
		"formatBytes": formatBytes,
	}).ParseFS(rootweb.FS, "templates/layout.html", "templates/dashboard.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.ExecuteTemplate(w, "layout", data)
}

func (s *Server) handlePage(tmplName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFS(rootweb.FS, "templates/layout.html", filepath.Join("templates", tmplName))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data := struct{ AuthEnabled bool }{AuthEnabled: s.cfg.Auth.Username != ""}
		if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	}
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	pageStr := r.URL.Query().Get("page")
	statusFilter := r.URL.Query().Get("status")

	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	page := 1
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	offset := (page - 1) * limit

	reports, total, err := db.GetJobReports(limit, offset, statusFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}
	
	data := struct {
		AuthEnabled  bool
		Reports      []db.JobReport
		Total        int
		CurrentPage  int
		TotalPages   int
		HasNext      bool
		HasPrev      bool
		PrevPage     int
		NextPage     int
		FilterStatus string
		Limit        int
	}{
		AuthEnabled:  s.cfg.Auth.Username != "",
		Reports:      reports,
		Total:        total,
		CurrentPage:  page,
		TotalPages:   totalPages,
		HasNext:      page < totalPages,
		HasPrev:      page > 1,
		PrevPage:     page - 1,
		NextPage:     page + 1,
		FilterStatus: statusFilter,
		Limit:        limit,
	}
	
	tmpl, err := template.New("layout").Funcs(template.FuncMap{
		"formatBytes": formatBytes,
	}).ParseFS(rootweb.FS, "templates/layout.html", "templates/history.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
