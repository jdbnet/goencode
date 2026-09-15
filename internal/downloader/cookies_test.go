package downloader

import (
	"os"
	"strings"
	"testing"

	"goencode/internal/config"
)

func TestValidateCookiesContent(t *testing.T) {
	valid := "# Netscape HTTP Cookie File\n.example.com\tTRUE\t/\tFALSE\t0\tSID\tvalue\n"
	if err := ValidateCookiesContent([]byte(valid)); err != nil {
		t.Fatalf("expected valid cookies: %v", err)
	}
	if err := ValidateCookiesContent([]byte("not cookies")); err == nil {
		t.Fatal("expected invalid cookies error")
	}
}

func TestCookiesStoreUploadAndEffectivePath(t *testing.T) {
	dir := t.TempDir()
	store := NewCookiesStore(config.DownloaderConfig{}, dir)

	data := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tFALSE\t0\tsession\tabc\n")
	if err := store.SaveUpload(data); err != nil {
		t.Fatal(err)
	}
	if store.EffectiveCookiesFile() != store.UploadPath() {
		t.Fatalf("effective = %q upload = %q", store.EffectiveCookiesFile(), store.UploadPath())
	}

	st := store.Status(NewClientWithOptions(ClientOptions{BinaryPath: "yt-dlp"}))
	client := NewClientWithOptions(ClientOptions{BinaryPath: "yt-dlp"})
	client.ApplyCookiesStore(store)
	st = store.Status(client)
	if !st.Configured || st.Source != "upload" {
		t.Fatalf("status = %#v", st)
	}

	if err := store.DeleteUpload(); err != nil {
		t.Fatal(err)
	}
	if store.EffectiveCookiesFile() != "" {
		t.Fatalf("expected empty effective path, got %q", store.EffectiveCookiesFile())
	}
}

func TestCookiesStoreConfigOverridesUpload(t *testing.T) {
	dir := t.TempDir()
	configFile := dir + "/config-cookies.txt"
	if err := os.WriteFile(configFile, []byte("# Netscape HTTP Cookie File\n"), 0600); err != nil {
		t.Fatal(err)
	}

	store := NewCookiesStore(config.DownloaderConfig{CookiesFile: configFile}, dir)
	if store.EffectiveCookiesFile() != configFile {
		t.Fatalf("effective = %q", store.EffectiveCookiesFile())
	}

	err := store.SaveUpload([]byte("# Netscape HTTP Cookie File\n"))
	if err == nil || !strings.Contains(err.Error(), "cookies_file is set in config") {
		t.Fatalf("expected config override error, got %v", err)
	}

	st := store.Status(NewClientWithOptions(ClientOptions{BinaryPath: "yt-dlp", CookiesFile: configFile}))
	if !st.Configured || !st.ConfigOverride || st.CanUpload {
		t.Fatalf("status = %#v", st)
	}
}

func TestDataDirectory(t *testing.T) {
	cfg := &config.Config{
		Database: config.DatabaseConfig{Driver: "sqlite", Path: "/var/lib/goencode/goencode.db"},
		Encoder:  config.EncoderConfig{TempDir: "/tmp/goencode"},
	}
	if got := DataDirectory(cfg); got != "/var/lib/goencode" {
		t.Fatalf("got %q", got)
	}
}
