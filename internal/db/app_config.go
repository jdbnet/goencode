package db

import "database/sql"

func GetAppConfig(key string) (string, error) {
	var v sql.NullString
	err := DB.QueryRow(`SELECT config_value FROM app_config WHERE config_key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !v.Valid {
		return "", nil
	}
	return v.String, nil
}

func SetAppConfig(key, value string) error {
	_, err := DB.Exec(`
		INSERT INTO app_config (config_key, config_value) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE config_value = VALUES(config_value)`, key, value)
	return err
}
