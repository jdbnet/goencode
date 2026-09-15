package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"goencode/internal/config"
)

const (
	uploadedCookiesName = "yt-dlp-cookies.txt"
	maxCookiesBytes     = 5 * 1024 * 1024
)

type CookiesStatus struct {
	Configured     bool      `json:"configured"`
	Source         string    `json:"source"`
	Size           int64     `json:"size,omitempty"`
	ModifiedAt     time.Time `json:"modified_at,omitempty"`
	ConfigOverride bool      `json:"config_override"`
	CanUpload      bool      `json:"can_upload"`
}

type CookiesStore struct {
	configFile   string
	browser      string
	uploadPath   string
}

func DataDirectory(cfg *config.Config) string {
	if cfg == nil {
		return "."
	}
	if cfg.Database.IsSQLite() {
		p := strings.TrimSpace(cfg.Database.Path)
		if p == "" {
			p = "goencode.db"
		}
		if abs, err := filepath.Abs(p); err == nil {
			return filepath.Dir(abs)
		}
	}
	if dir := strings.TrimSpace(cfg.Encoder.TempDir); dir != "" {
		return dir
	}
	return "."
}

func NewCookiesStore(cfg config.DownloaderConfig, dataDir string) *CookiesStore {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		dataDir = "."
	}
	return &CookiesStore{
		configFile: strings.TrimSpace(cfg.CookiesFile),
		browser:    strings.TrimSpace(cfg.CookiesFromBrowser),
		uploadPath: filepath.Join(dataDir, uploadedCookiesName),
	}
}

func (s *CookiesStore) UploadPath() string {
	if s == nil {
		return uploadedCookiesName
	}
	return s.uploadPath
}

func (s *CookiesStore) EffectiveCookiesFile() string {
	if s == nil {
		return ""
	}
	if s.configFile != "" {
		return s.configFile
	}
	if fileExists(s.uploadPath) {
		return s.uploadPath
	}
	return ""
}

func (s *CookiesStore) Status(client *Client) CookiesStatus {
	st := CookiesStatus{
		ConfigOverride: s != nil && s.configFile != "",
		CanUpload:      s != nil && s.configFile == "",
	}
	if client != nil && client.CookiesConfigured() {
		st.Configured = true
		if s.configFile != "" {
			st.Source = "config"
			s.fillFileMeta(&st, s.configFile)
		} else if fileExists(s.uploadPath) {
			st.Source = "upload"
			s.fillFileMeta(&st, s.uploadPath)
		} else if s.browser != "" {
			st.Source = "browser"
		}
	}
	return st
}

func (s *CookiesStore) fillFileMeta(st *CookiesStatus, path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	st.Size = info.Size()
	st.ModifiedAt = info.ModTime().UTC()
}

func (s *CookiesStore) SaveUpload(data []byte) error {
	if s == nil {
		return fmt.Errorf("cookies store unavailable")
	}
	if s.configFile != "" {
		return fmt.Errorf("cookies_file is set in config; remove it to use uploaded cookies")
	}
	if err := ValidateCookiesContent(data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.uploadPath), 0755); err != nil {
		return fmt.Errorf("create cookies directory: %w", err)
	}
	tmp := s.uploadPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write cookies file: %w", err)
	}
	if err := os.Rename(tmp, s.uploadPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("save cookies file: %w", err)
	}
	return nil
}

func (s *CookiesStore) DeleteUpload() error {
	if s == nil {
		return fmt.Errorf("cookies store unavailable")
	}
	if !fileExists(s.uploadPath) {
		return nil
	}
	if err := os.Remove(s.uploadPath); err != nil {
		return fmt.Errorf("remove cookies file: %w", err)
	}
	return nil
}

func ValidateCookiesContent(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("cookies file is empty")
	}
	if len(data) > maxCookiesBytes {
		return fmt.Errorf("cookies file is too large (max %d MB)", maxCookiesBytes/(1024*1024))
	}
	text := string(data)
	if strings.Contains(text, "# Netscape HTTP Cookie File") || strings.Contains(text, "# HTTP Cookie File") {
		return nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Count(line, "\t") >= 6 {
			return nil
		}
	}
	return fmt.Errorf("file does not look like a Netscape cookies.txt export")
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func (c *Client) ApplyCookiesStore(store *CookiesStore) {
	if c == nil || store == nil {
		return
	}
	c.CookiesFile = store.EffectiveCookiesFile()
}
