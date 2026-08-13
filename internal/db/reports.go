package db

import (
	"database/sql"
	"strings"
	"time"
)

type ReportTotals struct {
	Jobs             int     `json:"jobs"`
	Success          int     `json:"success"`
	Failed           int     `json:"failed"`
	Skipped          int     `json:"skipped"`
	SpaceSaved       int64   `json:"space_saved"`
	OriginalSize     int64   `json:"original_size"`
	EncodedSize      int64   `json:"encoded_size"`
	EncodeSeconds    float64 `json:"encode_seconds"`
	AvgEncodeSeconds float64 `json:"avg_encode_seconds"`
	CompressionPct   float64 `json:"compression_pct"`
}

type ReportBucket struct {
	Day        string `json:"day"`
	Jobs       int    `json:"jobs"`
	Success    int    `json:"success"`
	SpaceSaved int64  `json:"space_saved"`
}

type ReportGroup struct {
	Name       string `json:"name"`
	Jobs       int    `json:"jobs"`
	SpaceSaved int64  `json:"space_saved"`
}

type ReportTopFile struct {
	FilePath       string    `json:"file_path"`
	OriginalSize   int64     `json:"original_size"`
	EncodedSize    int64     `json:"encoded_size"`
	SizeSaved      int64     `json:"size_saved"`
	ProcessingTime float64   `json:"processing_time"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReportStats struct {
	Range       string          `json:"range"`
	Bucket      string          `json:"bucket"`
	Since       string          `json:"since"`
	Totals      ReportTotals    `json:"totals"`
	Daily       []ReportBucket  `json:"daily"`
	ByCodec     []ReportGroup   `json:"by_codec"`
	ByMediaType []ReportGroup   `json:"by_media_type"`
	ByFolder    []ReportGroup   `json:"by_folder"`
	TopSavers   []ReportTopFile `json:"top_savers"`
}

func GetReportStats(rangeKey string) (ReportStats, error) {
	now := time.Now().UTC()
	since, monthly, err := reportWindow(rangeKey, now)
	if err != nil {
		return ReportStats{}, err
	}

	stats := ReportStats{
		Range:       rangeKey,
		Bucket:      "day",
		Daily:       []ReportBucket{},
		ByCodec:     []ReportGroup{},
		ByMediaType: []ReportGroup{},
		ByFolder:    []ReportGroup{},
		TopSavers:   []ReportTopFile{},
	}
	if monthly {
		stats.Bucket = "month"
	}
	if !since.IsZero() {
		stats.Since = since.Format(time.RFC3339)
	}

	where := "1=1"
	args := []interface{}{}
	if !since.IsZero() {
		where += " AND created_at >= ?"
		args = append(args, since)
	}

	if err := scanReportTotals(&stats.Totals, where, args); err != nil {
		return stats, err
	}
	if stats.Totals.Success > 0 && stats.Totals.OriginalSize > 0 {
		stats.Totals.CompressionPct = (1 - float64(stats.Totals.EncodedSize)/float64(stats.Totals.OriginalSize)) * 100
		stats.Totals.AvgEncodeSeconds = stats.Totals.EncodeSeconds / float64(stats.Totals.Success)
	}

	daily, err := queryReportBuckets(where, args, monthly)
	if err != nil {
		return stats, err
	}
	from := since
	if from.IsZero() {
		from = earliestBucketTime(daily, now)
	}
	stats.Daily = fillReportBuckets(daily, from, now, monthly)

	stats.ByCodec, err = queryReportGroups(
		`SELECT COALESCE(NULLIF(video_codec, ''), 'libx265') AS name, COUNT(*), COALESCE(SUM(size_saved), 0)
		 FROM job_reports WHERE `+where+` AND status = 'success' AND media_type = 'video'
		 GROUP BY name ORDER BY COUNT(*) DESC`, args)
	if err != nil {
		return stats, err
	}

	stats.ByMediaType, err = queryReportGroups(
		`SELECT media_type AS name, COUNT(*), COALESCE(SUM(CASE WHEN status = 'success' THEN size_saved ELSE 0 END), 0)
		 FROM job_reports WHERE `+where+`
		 GROUP BY media_type ORDER BY COUNT(*) DESC`, args)
	if err != nil {
		return stats, err
	}

	stats.ByFolder, err = queryFolderGroups(where, args)
	if err != nil {
		return stats, err
	}

	stats.TopSavers, err = queryTopSavers(where, args)
	if err != nil {
		return stats, err
	}

	return stats, nil
}

func reportWindow(rangeKey string, now time.Time) (time.Time, bool, error) {
	now = now.UTC()
	switch rangeKey {
	case "7d":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -6), false, nil
	case "90d":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -89), false, nil
	case "all":
		var minCreated sql.NullTime
		err := DB.QueryRow(`SELECT MIN(created_at) FROM job_reports`).Scan(&minCreated)
		if err != nil && err != sql.ErrNoRows {
			return time.Time{}, false, err
		}
		if !minCreated.Valid {
			return time.Time{}, false, nil
		}
		start := minCreated.Time.UTC()
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		monthly := now.Sub(start) > 120*24*time.Hour
		if monthly {
			start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
		return start, monthly, nil
	default:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -29), false, nil
	}
}

func scanReportTotals(t *ReportTotals, where string, args []interface{}) error {
	err := DB.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'skipped' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'success' THEN size_saved ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'success' THEN original_size ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'success' THEN encoded_size ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'success' THEN processing_time ELSE 0 END), 0)
		FROM job_reports WHERE `+where, args...).Scan(
		&t.Jobs, &t.Success, &t.Failed, &t.Skipped,
		&t.SpaceSaved, &t.OriginalSize, &t.EncodedSize, &t.EncodeSeconds,
	)
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

func queryReportBuckets(where string, args []interface{}, monthly bool) ([]ReportBucket, error) {
	bucket := dateBucketSQL(monthly)
	rows, err := DB.Query(`
		SELECT `+bucket+` AS bucket,
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'success' THEN size_saved ELSE 0 END), 0)
		FROM job_reports WHERE `+where+`
		GROUP BY `+bucket+` ORDER BY bucket ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReportBucket
	for rows.Next() {
		var b ReportBucket
		if err := rows.Scan(&b.Day, &b.Jobs, &b.Success, &b.SpaceSaved); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func earliestBucketTime(buckets []ReportBucket, fallback time.Time) time.Time {
	if len(buckets) == 0 {
		return time.Date(fallback.Year(), fallback.Month(), fallback.Day(), 0, 0, 0, 0, time.UTC)
	}
	t, err := time.ParseInLocation("2006-01-02", buckets[0].Day, time.UTC)
	if err != nil {
		return fallback
	}
	return t
}

func fillReportBuckets(existing []ReportBucket, from, to time.Time, monthly bool) []ReportBucket {
	byDay := make(map[string]ReportBucket, len(existing))
	for _, b := range existing {
		byDay[b.Day] = b
	}
	var out []ReportBucket
	cur := from.UTC()
	if monthly {
		cur = time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, time.UTC)
	} else {
		cur = time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, time.UTC)
	}
	end := to.UTC()
	for !cur.After(end) {
		key := cur.Format("2006-01-02")
		if b, ok := byDay[key]; ok {
			out = append(out, b)
		} else {
			out = append(out, ReportBucket{Day: key})
		}
		if monthly {
			cur = cur.AddDate(0, 1, 0)
		} else {
			cur = cur.AddDate(0, 0, 1)
		}
	}
	return out
}

func queryReportGroups(q string, args []interface{}) ([]ReportGroup, error) {
	rows, err := DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReportGroup
	for rows.Next() {
		var g ReportGroup
		if err := rows.Scan(&g.Name, &g.Jobs, &g.SpaceSaved); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if out == nil {
		out = []ReportGroup{}
	}
	return out, rows.Err()
}

func queryFolderGroups(where string, args []interface{}) ([]ReportGroup, error) {
	folders, err := GetWatchFolders()
	if err != nil {
		return nil, err
	}
	out := make([]ReportGroup, 0, len(folders))
	for _, f := range folders {
		folderArgs := append([]interface{}{}, args...)
		folderArgs = append(folderArgs, f.FolderPath, f.FolderPath+"/%")
		var g ReportGroup
		g.Name = f.FolderPath
		err := DB.QueryRow(`
			SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'success' THEN size_saved ELSE 0 END), 0)
			FROM job_reports
			WHERE `+where+` AND (file_path = ? OR file_path LIKE ?)`, folderArgs...).Scan(&g.Jobs, &g.SpaceSaved)
		if err != nil {
			return nil, err
		}
		if g.Jobs > 0 {
			out = append(out, g)
		}
	}
	return out, nil
}

func queryTopSavers(where string, args []interface{}) ([]ReportTopFile, error) {
	rows, err := DB.Query(`
		SELECT file_path, original_size, encoded_size, size_saved, processing_time, created_at
		FROM job_reports
		WHERE `+where+` AND status = 'success'
		ORDER BY size_saved DESC
		LIMIT 8`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReportTopFile
	for rows.Next() {
		var t ReportTopFile
		if err := rows.Scan(&t.FilePath, &t.OriginalSize, &t.EncodedSize, &t.SizeSaved, &t.ProcessingTime, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []ReportTopFile{}
	}
	return out, rows.Err()
}

func NormalizeReportRange(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "7d", "90d", "all":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "30d"
	}
}
