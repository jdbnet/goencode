package web

import (
	"encoding/json"
	"net/http"

	"goencode/internal/db"
)

func (s *Server) handleGetReports(w http.ResponseWriter, r *http.Request) {
	rangeKey := db.NormalizeReportRange(r.URL.Query().Get("range"))
	stats, err := db.GetReportStats(rangeKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
