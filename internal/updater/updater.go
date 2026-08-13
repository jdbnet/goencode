package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	versionURL  = "https://apps.jdbnet.co.uk/goencode.version"
	checksumURL = "https://apps.jdbnet.co.uk/goencode.sha256"
	binaryURL   = "https://apps.jdbnet.co.uk/goencode"

	checkTimeout    = 10 * time.Second
	downloadTimeout = 2 * time.Minute
)

// MaybeUpdate checks for a newer binary and replaces the current process if one
// is available. A nil error means either no update was needed or the check was
// skipped. Failures are returned to the caller, which should continue startup.
func MaybeUpdate(currentVersion string, disabled bool) error {
	if shouldSkip(currentVersion, disabled) {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	if !dirWritable(filepath.Dir(exe)) {
		log.Printf("Skipping update: binary directory is not writable")
		return nil
	}

	client := &http.Client{Timeout: checkTimeout}
	remoteVersion, err := fetchText(client, versionURL)
	if err != nil {
		return fmt.Errorf("fetch latest version: %w", err)
	}
	remoteVersion = strings.TrimSpace(remoteVersion)

	newer, err := isNewer(remoteVersion, currentVersion)
	if err != nil {
		return fmt.Errorf("compare versions: %w", err)
	}
	if !newer {
		return nil
	}

	expectedHash, err := fetchText(client, checksumURL)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}
	expectedHash, err = parseSHA256(expectedHash)
	if err != nil {
		return fmt.Errorf("parse checksum: %w", err)
	}

	localHash, err := fileSHA256(exe)
	if err != nil {
		return fmt.Errorf("hash current binary: %w", err)
	}
	if localHash == expectedHash {
		return nil
	}

	log.Printf("Updating GoEncode from v%s to v%s", currentVersion, remoteVersion)

	downloadClient := &http.Client{Timeout: downloadTimeout}
	if err := downloadAndReplace(downloadClient, exe, expectedHash); err != nil {
		return err
	}

	log.Printf("Updated to v%s, restarting", remoteVersion)
	return syscall.Exec(exe, os.Args, os.Environ())
}

func shouldSkip(currentVersion string, disabled bool) bool {
	if disabled {
		return true
	}
	if envNoUpdate() {
		return true
	}
	if currentVersion == "" || currentVersion == "dev" {
		return true
	}
	if inContainer() {
		return true
	}
	return false
}

func envNoUpdate() bool {
	v := strings.TrimSpace(os.Getenv("GOENCODE_NO_UPDATE"))
	switch strings.ToLower(v) {
	case "1", "true":
		return true
	default:
		return false
	}
}

func inContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".goencode-upd-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func fetchText(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "goencode-updater")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

type semver struct {
	major, minor, patch int
}

func parseSemver(s string) (semver, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return semver{}, fmt.Errorf("empty version")
	}

	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return semver{}, fmt.Errorf("invalid version %q", s)
	}

	var v semver
	nums := []*int{&v.major, &v.minor, &v.patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, fmt.Errorf("invalid version %q", s)
		}
		*nums[i] = n
	}
	return v, nil
}

func compareSemver(a, b semver) int {
	if a.major != b.major {
		return a.major - b.major
	}
	if a.minor != b.minor {
		return a.minor - b.minor
	}
	return a.patch - b.patch
}

func isNewer(remote, current string) (bool, error) {
	r, err := parseSemver(remote)
	if err != nil {
		return false, fmt.Errorf("remote: %w", err)
	}
	c, err := parseSemver(current)
	if err != nil {
		return false, fmt.Errorf("current: %w", err)
	}
	return compareSemver(r, c) > 0, nil
}

func parseSHA256(s string) (string, error) {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum")
	}
	hash := strings.ToLower(fields[0])
	if len(hash) != 64 {
		return "", fmt.Errorf("invalid sha256 digest")
	}
	for _, c := range hash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("invalid sha256 digest")
		}
	}
	return hash, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func downloadAndReplace(client *http.Client, dest, expectedHash string) error {
	req, err := http.NewRequest(http.MethodGet, binaryURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "goencode-updater")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download binary: %s", resp.Status)
	}

	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".goencode-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		return fmt.Errorf("write binary: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedHash {
		return fmt.Errorf("checksum mismatch")
	}

	info, err := os.Stat(dest)
	if err != nil {
		return err
	}
	if err := tmp.Chmod(info.Mode()); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}
	ok = true
	return nil
}
