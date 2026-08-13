package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"goencode/internal/config"
	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(cfg *config.DatabaseConfig) error {
	if cfg == nil {
		return fmt.Errorf("database config is required")
	}
	if DB != nil {
		_ = DB.Close()
		DB = nil
	}

	switch cfg.Driver {
	case "sqlite":
		if err := openSQLite(cfg.Path); err != nil {
			return err
		}
	case "mysql":
		if err := openMySQL(cfg); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported database driver %q (use sqlite or mysql)", cfg.Driver)
	}

	return runMigrations()
}

func openSQLite(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "goencode.db"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sqlite path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return fmt.Errorf("create sqlite directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_busy_timeout=8000&_journal_mode=WAL&_foreign_keys=1&_synchronous=NORMAL&_time_format=datetime&_texttotime=1&_timezone=UTC", filepath.ToSlash(abs))
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return err
	}

	DB = conn
	driverName = "sqlite"
	log.Printf("Using SQLite at %s", abs)
	return nil
}

func openMySQL(cfg *config.DatabaseConfig) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(25)
	conn.SetConnMaxLifetime(5 * time.Minute)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return err
	}

	DB = conn
	driverName = "mysql"
	log.Println("Connected to MariaDB successfully")
	return nil
}

func runMigrations() error {
	queries := mysqlCreateTables
	if usingSQLite() {
		queries = sqliteCreateTables
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
	if err := ensureColumn("jobs", "force", "BOOLEAN NOT NULL DEFAULT FALSE"); err != nil {
		return err
	}
	if err := ensureColumn("job_reports", "ffmpeg_command", "TEXT"); err != nil {
		return err
	}
	if err := ensureFilePathIndexes(); err != nil {
		return err
	}

	log.Println("Database schemas initialized")
	return nil
}

func ensureFilePathIndexes() error {
	indexes := []struct {
		table string
		name  string
	}{
		{"jobs", "idx_jobs_file_path"},
		{"job_reports", "idx_job_reports_file_path"},
	}
	for _, idx := range indexes {
		if err := ensureIndex(idx.table, idx.name, fmt.Sprintf("CREATE INDEX `%s` ON `%s` (file_path)", idx.name, idx.table)); err != nil {
			return err
		}
	}
	return nil
}

func ensureIndex(table, name, ddl string) error {
	exists, err := indexExists(table, name)
	if err != nil {
		return fmt.Errorf("migration failed: check index %s.%s: %w", table, name, err)
	}
	if exists {
		return nil
	}
	if _, err := DB.Exec(ddl); err != nil {
		return fmt.Errorf("migration failed: add index %s: %w", name, err)
	}
	log.Printf("Migrated %s: added index %s", table, name)
	return nil
}

func ensureWatchFolderEnabledColumn() error {
	if err := ensureColumn("watch_folders", "enabled", "BOOLEAN NOT NULL DEFAULT TRUE"); err != nil {
		return err
	}
	if _, err := DB.Exec(`UPDATE watch_folders SET enabled = 1 WHERE enabled IS NULL`); err != nil {
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
		{"keep_extra_streams", "BOOLEAN NOT NULL DEFAULT TRUE"},
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
	exists, err := columnExists(table, name)
	if err != nil {
		return fmt.Errorf("migration failed: check %s.%s: %w", table, name, err)
	}
	if exists {
		return nil
	}
	_, err = DB.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", table, name, compatColumnDef(definition)))
	if err != nil {
		return fmt.Errorf("migration failed: add %s.%s: %w", table, name, err)
	}
	log.Printf("Migrated %s: added %s", table, name)
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
