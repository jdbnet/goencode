package db

import (
	"database/sql"
	"path/filepath"
	"strings"
)

const folderSelectCols = `id, folder_path, media_type, target_resolution, custom_ffmpeg_flags, enabled, video_codec, audio_codec, crf, preset, tune, profile, container, output_dir, delete_source, keep_original_if_larger, keep_extra_streams, created_at, updated_at`

const jobSelectCols = `id, file_path, media_type, status, priority, original_size, target_resolution, ffmpeg_flags, error_message, video_codec, audio_codec, crf, preset, tune, profile, container, output_dir, delete_source, keep_original_if_larger, keep_extra_streams, force, created_at, updated_at`

const reportSelectCols = `id, file_path, media_type, status, original_size, encoded_size, size_saved, processing_time, target_resolution, ffmpeg_flags, error_message, video_codec, audio_codec, crf, preset, tune, profile, container, output_dir, delete_source, keep_original_if_larger, keep_extra_streams, created_at`

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func applyEncodeNulls(es *EncodeSettings, videoCodec, audioCodec, crf, preset, tune, profile, container, outputDir sql.NullString, deleteSource, keepIfLarger, keepExtra bool) {
	es.VideoCodec = "libx265"
	if videoCodec.Valid && videoCodec.String != "" {
		es.VideoCodec = videoCodec.String
	}
	es.AudioCodec = "copy"
	if audioCodec.Valid && audioCodec.String != "" {
		es.AudioCodec = audioCodec.String
	}
	if crf.Valid {
		es.CRF = crf.String
	}
	if preset.Valid {
		es.Preset = preset.String
	}
	if tune.Valid {
		es.Tune = tune.String
	}
	if profile.Valid {
		es.Profile = profile.String
	}
	es.Container = "mkv"
	if container.Valid && container.String != "" {
		es.Container = container.String
	}
	if outputDir.Valid {
		es.OutputDir = outputDir.String
	}
	es.DeleteSource = deleteSource
	es.KeepOriginalIfLarger = keepIfLarger
	es.KeepExtraStreams = keepExtra
}

func encodeInsertArgs(es EncodeSettings) []interface{} {
	es.ApplyDefaults()
	return []interface{}{
		es.VideoCodec,
		es.AudioCodec,
		nullStr(es.CRF),
		nullStr(es.Preset),
		nullStr(es.Tune),
		nullStr(es.Profile),
		es.Container,
		nullStr(es.OutputDir),
		es.DeleteSource,
		es.KeepOriginalIfLarger,
		es.KeepExtraStreams,
	}
}

func scanWatchFolder(scan func(dest ...interface{}) error) (WatchFolder, error) {
	var f WatchFolder
	var targetRes, ffmpegFlags sql.NullString
	var videoCodec, audioCodec, crf, preset, tune, profile, container, outputDir sql.NullString
	err := scan(
		&f.ID, &f.FolderPath, &f.MediaType, &targetRes, &ffmpegFlags, &f.Enabled,
		&videoCodec, &audioCodec, &crf, &preset, &tune, &profile, &container, &outputDir,
		&f.DeleteSource, &f.KeepOriginalIfLarger, &f.KeepExtraStreams,
		&f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return f, err
	}
	if targetRes.Valid {
		f.TargetResolution = targetRes.String
	}
	if ffmpegFlags.Valid {
		f.CustomFFmpegFlags = ffmpegFlags.String
	}
	applyEncodeNulls(&f.EncodeSettings, videoCodec, audioCodec, crf, preset, tune, profile, container, outputDir, f.DeleteSource, f.KeepOriginalIfLarger, f.KeepExtraStreams)
	return f, nil
}

func scanJob(scan func(dest ...interface{}) error) (Job, error) {
	var j Job
	var targetRes, ffmpegFlags, errMsg sql.NullString
	var videoCodec, audioCodec, crf, preset, tune, profile, container, outputDir sql.NullString
	err := scan(
		&j.ID, &j.FilePath, &j.MediaType, &j.Status, &j.Priority, &j.OriginalSize, &targetRes, &ffmpegFlags, &errMsg,
		&videoCodec, &audioCodec, &crf, &preset, &tune, &profile, &container, &outputDir,
		&j.DeleteSource, &j.KeepOriginalIfLarger, &j.KeepExtraStreams, &j.Force,
		&j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return j, err
	}
	if targetRes.Valid {
		j.TargetResolution = targetRes.String
	}
	if ffmpegFlags.Valid {
		j.FFmpegFlags = ffmpegFlags.String
	}
	if errMsg.Valid {
		j.ErrorMessage = errMsg.String
	}
	applyEncodeNulls(&j.EncodeSettings, videoCodec, audioCodec, crf, preset, tune, profile, container, outputDir, j.DeleteSource, j.KeepOriginalIfLarger, j.KeepExtraStreams)
	return j, nil
}

func scanJobReport(scan func(dest ...interface{}) error) (JobReport, error) {
	var r JobReport
	var targetRes, ffmpegFlags, errMsg sql.NullString
	var videoCodec, audioCodec, crf, preset, tune, profile, container, outputDir sql.NullString
	err := scan(
		&r.ID, &r.FilePath, &r.MediaType, &r.Status, &r.OriginalSize, &r.EncodedSize, &r.SizeSaved, &r.ProcessingTime, &targetRes, &ffmpegFlags, &errMsg,
		&videoCodec, &audioCodec, &crf, &preset, &tune, &profile, &container, &outputDir,
		&r.DeleteSource, &r.KeepOriginalIfLarger, &r.KeepExtraStreams,
		&r.CreatedAt,
	)
	if err != nil {
		return r, err
	}
	if targetRes.Valid {
		r.TargetResolution = targetRes.String
	}
	if ffmpegFlags.Valid {
		r.FFmpegFlags = ffmpegFlags.String
	}
	if errMsg.Valid {
		r.ErrorMessage = errMsg.String
	}
	applyEncodeNulls(&r.EncodeSettings, videoCodec, audioCodec, crf, preset, tune, profile, container, outputDir, r.DeleteSource, r.KeepOriginalIfLarger, r.KeepExtraStreams)
	return r, nil
}

func GetWatchFolders() ([]WatchFolder, error) {
	rows, err := DB.Query(`SELECT ` + folderSelectCols + ` FROM watch_folders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []WatchFolder
	for rows.Next() {
		f, err := scanWatchFolder(rows.Scan)
		if err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

func AddWatchFolder(f WatchFolder) error {
	f.EncodeSettings.ApplyDefaults()
	args := []interface{}{f.FolderPath, f.MediaType, nullStr(f.TargetResolution), nullStr(f.CustomFFmpegFlags), f.Enabled}
	args = append(args, encodeInsertArgs(f.EncodeSettings)...)
	_, err := DB.Exec(`INSERT INTO watch_folders (folder_path, media_type, target_resolution, custom_ffmpeg_flags, enabled, video_codec, audio_codec, crf, preset, tune, profile, container, output_dir, delete_source, keep_original_if_larger, keep_extra_streams) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
	return err
}

func UpdateWatchFolder(f WatchFolder) error {
	f.EncodeSettings.ApplyDefaults()
	args := []interface{}{f.FolderPath, f.MediaType, nullStr(f.TargetResolution), nullStr(f.CustomFFmpegFlags)}
	args = append(args, encodeInsertArgs(f.EncodeSettings)...)
	args = append(args, f.ID)
	_, err := DB.Exec(`UPDATE watch_folders SET folder_path = ?, media_type = ?, target_resolution = ?, custom_ffmpeg_flags = ?, video_codec = ?, audio_codec = ?, crf = ?, preset = ?, tune = ?, profile = ?, container = ?, output_dir = ?, delete_source = ?, keep_original_if_larger = ?, keep_extra_streams = ? WHERE id = ?`, args...)
	return err
}

func DeleteWatchFolder(id int) error {
	_, err := DB.Exec(`DELETE FROM watch_folders WHERE id = ?`, id)
	return err
}

func GetWatchFolderByID(id int) (WatchFolder, error) {
	return scanWatchFolder(DB.QueryRow(`SELECT `+folderSelectCols+` FROM watch_folders WHERE id = ?`, id).Scan)
}

func SetWatchFolderEnabled(id int, enabled bool) error {
	_, err := DB.Exec(`UPDATE watch_folders SET enabled = ? WHERE id = ?`, enabled, id)
	return err
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func pathPrefixFilter(folderPath string) (exact, like string) {
	exact = filepath.Clean(strings.TrimSpace(folderPath))
	return exact, escapeLike(exact) + "/%"
}

func collectFilePaths(query string, args ...interface{}) (map[string]struct{}, error) {
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]struct{})
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths[filepath.Clean(p)] = struct{}{}
	}
	return paths, rows.Err()
}

func KnownFilePathsUnder(folderPath string) (map[string]struct{}, error) {
	exact, like := pathPrefixFilter(folderPath)
	return collectFilePaths(
		`SELECT file_path FROM jobs WHERE file_path = ? OR file_path LIKE ? ESCAPE '\\'
		 UNION
		 SELECT file_path FROM job_reports WHERE file_path = ? OR file_path LIKE ? ESCAPE '\\'`,
		exact, like, exact, like,
	)
}

func QueuedFilePathsUnder(folderPath string) (map[string]struct{}, error) {
	exact, like := pathPrefixFilter(folderPath)
	return collectFilePaths(
		`SELECT file_path FROM jobs WHERE file_path = ? OR file_path LIKE ? ESCAPE '\\'`,
		exact, like,
	)
}

func PathUnderFolder(path, folder string) bool {
	path = filepath.Clean(path)
	folder = filepath.Clean(strings.TrimSpace(folder))
	if path == folder {
		return true
	}
	if folder == string(filepath.Separator) {
		return strings.HasPrefix(path, folder)
	}
	return strings.HasPrefix(path, folder+string(filepath.Separator))
}

func FolderContaining(filePath string) (WatchFolder, bool) {
	filePath = filepath.Clean(filePath)
	folders, err := GetWatchFolders()
	if err != nil {
		return WatchFolder{}, false
	}
	var best WatchFolder
	bestLen := -1
	for _, f := range folders {
		if !f.Enabled {
			continue
		}
		folderPath := filepath.Clean(strings.TrimSpace(f.FolderPath))
		if !PathUnderFolder(filePath, folderPath) {
			continue
		}
		if len(folderPath) > bestLen {
			best = f
			bestLen = len(folderPath)
		}
	}
	return best, bestLen >= 0
}

func DeleteJobsUnderPath(folderPath string) (int64, error) {
	exact, like := pathPrefixFilter(folderPath)
	res, err := DB.Exec(`DELETE FROM jobs WHERE file_path = ? OR file_path LIKE ? ESCAPE '\\'`, exact, like)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func DeleteJobReportsUnderPath(folderPath string) (int64, error) {
	exact, like := pathPrefixFilter(folderPath)
	res, err := DB.Exec(`DELETE FROM job_reports WHERE file_path = ? OR file_path LIKE ? ESCAPE '\\'`, exact, like)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func AddJob(j Job) error {
	j.EncodeSettings.ApplyDefaults()
	args := []interface{}{j.FilePath, j.MediaType, j.Priority, nullStr(j.TargetResolution), nullStr(j.FFmpegFlags), j.OriginalSize}
	args = append(args, encodeInsertArgs(j.EncodeSettings)...)
	args = append(args, j.Force)
	_, err := DB.Exec(`INSERT INTO jobs (file_path, media_type, priority, target_resolution, ffmpeg_flags, original_size, video_codec, audio_codec, crf, preset, tune, profile, container, output_dir, delete_source, keep_original_if_larger, keep_extra_streams, force) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
	return err
}

func GetPendingJobs() ([]Job, error) {
	rows, err := DB.Query(`SELECT ` + jobSelectCols + ` FROM jobs ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows.Scan)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func GetJobsPaginated(limit, offset int) ([]Job, int, error) {
	var total int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := DB.Query(`SELECT `+jobSelectCols+` FROM jobs ORDER BY priority DESC, created_at ASC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		j, err := scanJob(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	return jobs, total, rows.Err()
}

func UpdateJobStatus(id int, status, errMsg string) error {
	_, err := DB.Exec(`UPDATE jobs SET status = ?, error_message = ?, updated_at = UTC_TIMESTAMP(6) WHERE id = ?`, status, nullStr(errMsg), id)
	return err
}

func ClaimJob(id int) (bool, error) {
	res, err := DB.Exec(`UPDATE jobs SET status = 'processing', error_message = NULL, updated_at = UTC_TIMESTAMP(6) WHERE id = ? AND status = 'pending'`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
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

func IsFileQueued(filePath string) (bool, error) {
	var exists int
	err := DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM jobs WHERE file_path = ?)`, filePath).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func IsFileAlreadyProcessedOrQueued(filePath string) (bool, error) {
	var exists int
	err := DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM jobs WHERE file_path = ?
			UNION ALL
			SELECT 1 FROM job_reports WHERE file_path = ?
		)`, filePath, filePath).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func AddJobReport(j Job, status string, encodedSize int64, sizeSaved int64, processingTime float64) error {
	j.EncodeSettings.ApplyDefaults()
	args := []interface{}{j.FilePath, j.MediaType, status, j.OriginalSize, encodedSize, sizeSaved, processingTime, nullStr(j.TargetResolution), nullStr(j.FFmpegFlags), nullStr(j.ErrorMessage)}
	args = append(args, encodeInsertArgs(j.EncodeSettings)...)
	_, err := DB.Exec(`INSERT INTO job_reports (file_path, media_type, status, original_size, encoded_size, size_saved, processing_time, target_resolution, ffmpeg_flags, error_message, video_codec, audio_codec, crf, preset, tune, profile, container, output_dir, delete_source, keep_original_if_larger, keep_extra_streams) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
	return err
}

func GetJobReports(limit, offset int, statusFilter string, folderFilter string) ([]JobReport, int, error) {
	var total int
	queryArgs := []interface{}{}
	countQuery := `SELECT COUNT(*) FROM job_reports`
	selectQuery := `SELECT ` + reportSelectCols + ` FROM job_reports`

	whereClauses := []string{}

	if statusFilter != "" && statusFilter != "all" {
		if statusFilter == "exclude_skipped" {
			whereClauses = append(whereClauses, `status != 'skipped'`)
		} else {
			whereClauses = append(whereClauses, `status = ?`)
			queryArgs = append(queryArgs, statusFilter)
		}
	}

	if folderFilter != "" && folderFilter != "all" {
		whereClauses = append(whereClauses, `(file_path LIKE ? OR file_path = ?)`)
		queryArgs = append(queryArgs, folderFilter+"/%", folderFilter)
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = " WHERE " + strings.Join(whereClauses, " AND ")
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
		r, err := scanJobReport(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		reports = append(reports, r)
	}
	return reports, total, rows.Err()
}

func GetJobReportByID(id int) (JobReport, error) {
	return scanJobReport(DB.QueryRow(`SELECT `+reportSelectCols+` FROM job_reports WHERE id = ?`, id).Scan)
}

func DeleteJobReport(id int) error {
	_, err := DB.Exec(`DELETE FROM job_reports WHERE id = ?`, id)
	return err
}

type DashboardStats struct {
	TotalSavedSpace int64
	FilesEncoded    int
	QueueLength     int
}

func GetDashboardStats() (DashboardStats, error) {
	var stats DashboardStats

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

	err = DB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&stats.QueueLength)
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	return stats, nil
}
