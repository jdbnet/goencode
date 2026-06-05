package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

func getEnvStr(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

type ServerConfig struct {
	Port       int    `yaml:"port"`
	ListenAddr string `yaml:"listen_addr"`
	TimeZone   string `yaml:"timezone"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type EncoderConfig struct {
	FFmpegPath string `yaml:"ffmpeg_path"`
	TempDir    string `yaml:"temp_dir"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Database DatabaseConfig `yaml:"database"`
	Encoder  EncoderConfig  `yaml:"encoder"`
	Logging  LoggingConfig  `yaml:"logging"`
}

func LoadConfig(path string) (*Config, error) {
	var cfg Config

	// Try reading from file first (it's okay if it doesn't exist for env-only deployments)
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("could not parse config file: %w", err)
		}
	}

	// Override with env variables
	cfg.Server.Port = getEnvInt("GOENCODE_PORT", cfg.Server.Port)
	cfg.Server.ListenAddr = getEnvStr("GOENCODE_LISTEN_ADDR", cfg.Server.ListenAddr)
	cfg.Auth.Username = getEnvStr("GOENCODE_AUTH_USER", cfg.Auth.Username)
	cfg.Auth.Password = getEnvStr("GOENCODE_AUTH_PASS", cfg.Auth.Password)
	cfg.Database.Host = getEnvStr("GOENCODE_DB_HOST", cfg.Database.Host)
	cfg.Database.Port = getEnvInt("GOENCODE_DB_PORT", cfg.Database.Port)
	cfg.Database.User = getEnvStr("GOENCODE_DB_USER", cfg.Database.User)
	cfg.Database.Password = getEnvStr("GOENCODE_DB_PASS", cfg.Database.Password)
	cfg.Database.Name = getEnvStr("GOENCODE_DB_NAME", cfg.Database.Name)
	cfg.Encoder.FFmpegPath = getEnvStr("GOENCODE_FFMPEG_PATH", cfg.Encoder.FFmpegPath)
	cfg.Encoder.TempDir = getEnvStr("GOENCODE_TEMP_DIR", cfg.Encoder.TempDir)
	cfg.Logging.Level = getEnvStr("GOENCODE_LOG_LEVEL", cfg.Logging.Level)

	// Defaults if missing entirely
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = "0.0.0.0"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 3306
	}

	if envTZ := os.Getenv("TZ"); envTZ != "" {
		cfg.Server.TimeZone = envTZ
	}

	// Apply timezone if set
	if cfg.Server.TimeZone != "" {
		os.Setenv("TZ", cfg.Server.TimeZone)
	}

	return &cfg, nil
}
