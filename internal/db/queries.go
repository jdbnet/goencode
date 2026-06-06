package db

import (
	"database/sql"
)

// Watch Folders

func GetWatchFolders() ([]WatchFolder, error) {
	rows, err := DB.Query(`SELECT id, folder_path, media_type, target_resolution, custom_ffmpeg_flags, enabled, created_at, updated_at FROM watch_folders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []WatchFolder
	for rows.Next() {
		var f WatchFolder
		var targetRes, ffmpegFlags sql.NullString
		if err := rows.Scan(&f.ID, &f.FolderPath, &f.MediaType, &targetRes, &ffmpegFlags, &f.Enabled, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		if targetRes.Valid {
			f.TargetResolution = targetRes.String
		}
		if ffmpegFlags.Valid {
			f.CustomFFmpegFlags = ffmpegFlags.String
		}
		folders = append(folders, f)
	}
	return folders, nil
}

func AddWatchFolder(f WatchFolder) error {
	_, err := DB.Exec(`INSERT INTO watch_folders (folder_path, media_type, target_resolution, custom_ffmpeg_flags, enabled) VALUES (?, ?, ?, ?, ?)`,
		f.FolderPath, f.MediaType, nullStr(f.TargetResolution), nullStr(f.CustomFFmpegFlags), f.Enabled)
	return err
}

func DeleteWatchFolder(id int) error {
	_, err := DB.Exec(`DELETE FROM watch_folders WHERE id = ?`, id)
	return err
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// Jobs

func AddJob(filePath, mediaType string, priority int, targetRes, ffmpegFlags string, originalSize int64) error {
	_, err := DB.Exec(`INSERT INTO jobs (file_path, media_type, priority, target_resolution, ffmpeg_flags, original_size) VALUES (?, ?, ?, ?, ?, ?)`,
		filePath, mediaType, priority, nullStr(targetRes), nullStr(ffmpegFlags), originalSize)
	return err
}

func GetPendingJobs() ([]Job, error) {
	rows, err := DB.Query(`SELECT id, file_path, media_type, status, priority, original_size, target_resolution, ffmpeg_flags, error_message, created_at, updated_at FROM jobs ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var targetRes, ffmpegFlags, errMsg sql.NullString
		if err := rows.Scan(&j.ID, &j.FilePath, &j.MediaType, &j.Status, &j.Priority, &j.OriginalSize, &targetRes, &ffmpegFlags, &errMsg, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		if targetRes.Valid { j.TargetResolution = targetRes.String }
		if ffmpegFlags.Valid { j.FFmpegFlags = ffmpegFlags.String }
		if errMsg.Valid { j.ErrorMessage = errMsg.String }
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func UpdateJobStatus(id int, status, errMsg string) error {
	_, err := DB.Exec(`UPDATE jobs SET status = ?, error_message = ? WHERE id = ?`, status, nullStr(errMsg), id)
	return err
}

func BumpJobPriority(id int) error {
	_, err := DB.Exec(`UPDATE jobs SET priority = priority + 1 WHERE id = ?`, id)
	return err
}

func DeleteJob(id int) error {
	_, err := DB.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

func MarkProcessingAsFailed() error {
	_, err := DB.Exec(`UPDATE jobs SET status = 'failed', error_message = 'Interrupted by server restart' WHERE status = 'processing'`)
	return err
}

func IsFileAlreadyProcessedOrQueued(filePath string) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE file_path = ?`, filePath).Scan(&count)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	
	err = DB.QueryRow(`SELECT COUNT(*) FROM job_reports WHERE file_path = ?`, filePath).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Job Reports

func AddJobReport(j Job, status string, encodedSize int64, sizeSaved int64, processingTime float64) error {
	_, err := DB.Exec(`INSERT INTO job_reports (file_path, media_type, status, original_size, encoded_size, size_saved, processing_time, target_resolution, ffmpeg_flags, error_message) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.FilePath, j.MediaType, status, j.OriginalSize, encodedSize, sizeSaved, processingTime, nullStr(j.TargetResolution), nullStr(j.FFmpegFlags), nullStr(j.ErrorMessage))
	return err
}

func GetJobReports(limit, offset int, statusFilter string) ([]JobReport, int, error) {
	var total int
	queryArgs := []interface{}{}
	countQuery := `SELECT COUNT(*) FROM job_reports`
	selectQuery := `SELECT id, file_path, media_type, status, original_size, encoded_size, size_saved, processing_time, target_resolution, ffmpeg_flags, error_message, created_at FROM job_reports`
	whereClause := ""

	if statusFilter != "" && statusFilter != "all" {
		if statusFilter == "exclude_skipped" {
			whereClause = ` WHERE status != 'skipped'`
		} else {
			whereClause = ` WHERE status = ?`
			queryArgs = append(queryArgs, statusFilter)
		}
	}

	if err := DB.QueryRow(countQuery+whereClause, queryArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	selectQuery += whereClause + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	queryArgs = append(queryArgs, limit, offset)

	rows, err := DB.Query(selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []JobReport
	for rows.Next() {
		var r JobReport
		var targetRes, ffmpegFlags, errMsg sql.NullString
		if err := rows.Scan(&r.ID, &r.FilePath, &r.MediaType, &r.Status, &r.OriginalSize, &r.EncodedSize, &r.SizeSaved, &r.ProcessingTime, &targetRes, &ffmpegFlags, &errMsg, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		if targetRes.Valid { r.TargetResolution = targetRes.String }
		if ffmpegFlags.Valid { r.FFmpegFlags = ffmpegFlags.String }
		if errMsg.Valid { r.ErrorMessage = errMsg.String }
		reports = append(reports, r)
	}
	return reports, total, nil
}

type DashboardStats struct {
	TotalSavedSpace int64
	FilesEncoded    int
	QueueLength     int
}

func GetDashboardStats() (DashboardStats, error) {
	var stats DashboardStats

	// Total saved space and files encoded
	err := DB.QueryRow(`
		SELECT 
			COALESCE(SUM(size_saved), 0) as saved, 
			COUNT(*) as count 
		FROM job_reports 
		WHERE status = 'success'
	`).Scan(&stats.TotalSavedSpace, &stats.FilesEncoded)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	// Queue length
	err = DB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&stats.QueueLength)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	return stats, nil
}
