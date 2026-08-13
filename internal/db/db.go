package db

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"goencode/internal/config"
)

var DB *sql.DB

func Init(cfg *config.DatabaseConfig) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		return err
	}

	// Set connection pool limits
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(25)
	DB.SetConnMaxLifetime(5 * time.Minute)

	if err = DB.Ping(); err != nil {
		return err
	}

	log.Println("Connected to MariaDB successfully")

	return runMigrations()
}

func runMigrations() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS watch_folders (
			id INT AUTO_INCREMENT PRIMARY KEY,
			folder_path VARCHAR(500) NOT NULL UNIQUE,
			media_type ENUM('video', 'audio') NOT NULL,
			target_resolution VARCHAR(20),
			custom_ffmpeg_flags TEXT,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			video_codec VARCHAR(32) NOT NULL DEFAULT 'libx265',
			audio_codec VARCHAR(32) NOT NULL DEFAULT 'copy',
			crf VARCHAR(8) NULL,
			preset VARCHAR(32) NULL,
			tune VARCHAR(32) NULL,
			profile VARCHAR(32) NULL,
			container VARCHAR(8) NOT NULL DEFAULT 'mkv',
			output_dir VARCHAR(500) NULL,
			delete_source BOOLEAN NOT NULL DEFAULT FALSE,
			keep_original_if_larger BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id INT AUTO_INCREMENT PRIMARY KEY,
			file_path VARCHAR(500) NOT NULL,
			media_type ENUM('video', 'audio') NOT NULL,
			status ENUM('pending', 'processing', 'failed') NOT NULL DEFAULT 'pending',
			priority INT NOT NULL DEFAULT 0,
			original_size BIGINT DEFAULT 0,
			target_resolution VARCHAR(20),
			ffmpeg_flags TEXT,
			error_message TEXT,
			video_codec VARCHAR(32) NOT NULL DEFAULT 'libx265',
			audio_codec VARCHAR(32) NOT NULL DEFAULT 'copy',
			crf VARCHAR(8) NULL,
			preset VARCHAR(32) NULL,
			tune VARCHAR(32) NULL,
			profile VARCHAR(32) NULL,
			container VARCHAR(8) NOT NULL DEFAULT 'mkv',
			output_dir VARCHAR(500) NULL,
			delete_source BOOLEAN NOT NULL DEFAULT FALSE,
			keep_original_if_larger BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS job_reports (
			id INT AUTO_INCREMENT PRIMARY KEY,
			file_path VARCHAR(500) NOT NULL,
			media_type ENUM('video', 'audio') NOT NULL,
			status ENUM('success', 'failed', 'skipped') NOT NULL,
			original_size BIGINT NOT NULL DEFAULT 0,
			encoded_size BIGINT NOT NULL DEFAULT 0,
			size_saved BIGINT NOT NULL DEFAULT 0,
			processing_time DECIMAL(10,2) DEFAULT 0,
			target_resolution VARCHAR(20),
			ffmpeg_flags TEXT,
			error_message TEXT,
			video_codec VARCHAR(32) NOT NULL DEFAULT 'libx265',
			audio_codec VARCHAR(32) NOT NULL DEFAULT 'copy',
			crf VARCHAR(8) NULL,
			preset VARCHAR(32) NULL,
			tune VARCHAR(32) NULL,
			profile VARCHAR(32) NULL,
			container VARCHAR(8) NOT NULL DEFAULT 'mkv',
			output_dir VARCHAR(500) NULL,
			delete_source BOOLEAN NOT NULL DEFAULT FALSE,
			keep_original_if_larger BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS app_config (
			config_key VARCHAR(100) PRIMARY KEY,
			config_value TEXT
		)`,
	}

	for _, q := range queries {
		if _, err := DB.Exec(q); err != nil {
			return fmt.Errorf("migration failed: %w\nQuery: %s", err, q)
		}
	}

	if err := ensureWatchFolderEnabledColumn(); err != nil {
		return err
	}
	if err := ensureEncodeSettingsColumns(); err != nil {
		return err
	}

	log.Println("Database schemas initialized")
	return nil
}

func ensureWatchFolderEnabledColumn() error {
	var count int
	err := DB.QueryRow(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'watch_folders'
		  AND COLUMN_NAME = 'enabled'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("migration failed: check enabled column: %w", err)
	}

	if count == 0 {
		_, err = DB.Exec(`ALTER TABLE watch_folders ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE`)
		if err != nil {
			return fmt.Errorf("migration failed: add enabled column: %w", err)
		}
		log.Println("Migrated watch_folders: enabled column added, existing folders enabled")
	}

	if _, err := DB.Exec(`UPDATE watch_folders SET enabled = TRUE WHERE enabled IS NULL`); err != nil {
		return fmt.Errorf("migration failed: backfill enabled column: %w", err)
	}

	return nil
}

func ensureEncodeSettingsColumns() error {
	tables := []string{"watch_folders", "jobs", "job_reports"}
	columns := []struct {
		name string
		def  string
	}{
		{"video_codec", "VARCHAR(32) NOT NULL DEFAULT 'libx265'"},
		{"audio_codec", "VARCHAR(32) NOT NULL DEFAULT 'copy'"},
		{"crf", "VARCHAR(8) NULL"},
		{"preset", "VARCHAR(32) NULL"},
		{"tune", "VARCHAR(32) NULL"},
		{"profile", "VARCHAR(32) NULL"},
		{"container", "VARCHAR(8) NOT NULL DEFAULT 'mkv'"},
		{"output_dir", "VARCHAR(500) NULL"},
		{"delete_source", "BOOLEAN NOT NULL DEFAULT FALSE"},
		{"keep_original_if_larger", "BOOLEAN NOT NULL DEFAULT FALSE"},
	}
	for _, table := range tables {
		for _, col := range columns {
			if err := ensureColumn(table, col.name, col.def); err != nil {
				return err
			}
		}
	}
	return backfillCRFPresetFromFlags()
}

func ensureColumn(table, name, definition string) error {
	var count int
	err := DB.QueryRow(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND COLUMN_NAME = ?
	`, table, name).Scan(&count)
	if err != nil {
		return fmt.Errorf("migration failed: check %s.%s: %w", table, name, err)
	}
	if count == 0 {
		_, err = DB.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", table, name, definition))
		if err != nil {
			return fmt.Errorf("migration failed: add %s.%s: %w", table, name, err)
		}
		log.Printf("Migrated %s: added %s", table, name)
	}
	return nil
}

var (
	crfFlagRe    = regexp.MustCompile(`(?:^|\s)-crf\s+(\S+)`)
	presetFlagRe = regexp.MustCompile(`(?:^|\s)-preset\s+(\S+)`)
)

func extractCRFPreset(flags string) (crf, preset, rest string) {
	rest = flags
	if m := crfFlagRe.FindStringSubmatch(rest); len(m) > 1 {
		crf = m[1]
		rest = crfFlagRe.ReplaceAllString(rest, " ")
	}
	if m := presetFlagRe.FindStringSubmatch(rest); len(m) > 1 {
		preset = m[1]
		rest = presetFlagRe.ReplaceAllString(rest, " ")
	}
	rest = strings.Join(strings.Fields(rest), " ")
	return crf, preset, rest
}

func backfillCRFPresetFromFlags() error {
	rows, err := DB.Query(`SELECT id, custom_ffmpeg_flags, crf, preset FROM watch_folders`)
	if err != nil {
		return fmt.Errorf("migration failed: list folders for flag backfill: %w", err)
	}
	defer rows.Close()

	type row struct {
		id     int
		flags  sql.NullString
		crf    sql.NullString
		preset sql.NullString
	}
	var folders []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.flags, &r.crf, &r.preset); err != nil {
			return fmt.Errorf("migration failed: scan folder for flag backfill: %w", err)
		}
		folders = append(folders, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range folders {
		flags := ""
		if r.flags.Valid {
			flags = r.flags.String
		}
		if strings.TrimSpace(flags) == "" {
			continue
		}
		extractedCRF, extractedPreset, rest := extractCRFPreset(flags)
		crf := ""
		if r.crf.Valid {
			crf = r.crf.String
		}
		preset := ""
		if r.preset.Valid {
			preset = r.preset.String
		}
		if crf == "" {
			crf = extractedCRF
		}
		if preset == "" {
			preset = extractedPreset
		}
		if crf == "" && preset == "" && rest == strings.TrimSpace(flags) {
			continue
		}
		_, err := DB.Exec(`UPDATE watch_folders SET crf = ?, preset = ?, custom_ffmpeg_flags = ? WHERE id = ?`,
			nullStr(crf), nullStr(preset), nullStr(rest), r.id)
		if err != nil {
			return fmt.Errorf("migration failed: backfill folder %d flags: %w", r.id, err)
		}
	}
	return nil
}
