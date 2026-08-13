package db

import (
	"fmt"
	"strings"
)

var driverName = "mysql"

func usingSQLite() bool {
	return driverName == "sqlite"
}

func nowUTCExpr() string {
	if usingSQLite() {
		return "datetime('now')"
	}
	return "UTC_TIMESTAMP(6)"
}

func likeEscapeClause() string {
	if usingSQLite() {
		return `ESCAPE '\'`
	}
	return `ESCAPE '\\'`
}

func dateBucketSQL(monthly bool) string {
	layout := "%Y-%m-%d"
	if monthly {
		layout = "%Y-%m-01"
	}
	if usingSQLite() {
		return "strftime('" + layout + "', created_at)"
	}
	return "DATE_FORMAT(created_at, '" + layout + "')"
}

func upsertAppConfigSQL() string {
	if usingSQLite() {
		return `INSERT INTO app_config (config_key, config_value) VALUES (?, ?)
			ON CONFLICT(config_key) DO UPDATE SET config_value = excluded.config_value`
	}
	return `INSERT INTO app_config (config_key, config_value) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE config_value = VALUES(config_value)`
}

func compatColumnDef(def string) string {
	if !usingSQLite() {
		return def
	}
	repl := []struct{ old, new string }{
		{"BOOLEAN NOT NULL DEFAULT FALSE", "INTEGER NOT NULL DEFAULT 0"},
		{"BOOLEAN NOT NULL DEFAULT TRUE", "INTEGER NOT NULL DEFAULT 1"},
		{"BOOLEAN", "INTEGER"},
		{"VARCHAR(500)", "TEXT"},
		{"VARCHAR(32)", "TEXT"},
		{"VARCHAR(20)", "TEXT"},
		{"VARCHAR(8)", "TEXT"},
	}
	for _, r := range repl {
		def = strings.ReplaceAll(def, r.old, r.new)
	}
	return def
}

func columnExists(table, name string) (bool, error) {
	if usingSQLite() {
		rows, err := DB.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return false, err
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var colName, colType string
			var notNull int
			var dflt interface{}
			var pk int
			if err := rows.Scan(&cid, &colName, &colType, &notNull, &dflt, &pk); err != nil {
				return false, err
			}
			if strings.EqualFold(colName, name) {
				return true, rows.Err()
			}
		}
		return false, rows.Err()
	}

	var count int
	err := DB.QueryRow(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND COLUMN_NAME = ?
	`, table, name).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func indexExists(table, name string) (bool, error) {
	if usingSQLite() {
		var count int
		err := DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count)
		if err != nil {
			return false, err
		}
		return count > 0, nil
	}

	var count int
	err := DB.QueryRow(`
		SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND INDEX_NAME = ?
	`, table, name).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
