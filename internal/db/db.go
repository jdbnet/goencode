package db

import (
	"database/sql"
	"fmt"
	"log"
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
