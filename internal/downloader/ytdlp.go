package downloader

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Format struct {
	ID         string `json:"format_id"`
	Ext        string `json:"ext"`
	Resolution string `json:"resolution"`
	Note       string `json:"format_note"`
	VCodec     string `json:"vcodec"`
	ACodec     string `json:"acodec"`
	FileSize   int64  `json:"filesize,omitempty"`
}

type ProbeResult struct {
	Title      string   `json:"title"`
	Duration   float64  `json:"duration"`
	Thumbnail  string   `json:"thumbnail"`
	Uploader   string   `json:"uploader"`
	Extractor  string   `json:"extractor"`
	Formats    []Format `json:"formats"`
	Suggested  []Preset `json:"suggested"`
	DefaultExt string   `json:"default_ext"`
}

type Preset struct {
	Label     string `json:"label"`
	FormatID  string `json:"format_id"`
	Group     string `json:"group"`
}

type DownloadOptions struct {
	URL        string
	FormatID   string
	OutputDir  string
	Filename   string
	ProgressFn func(percent float64)
}

type Client struct {
	BinaryPath         string
	CookiesFile        string
	CookiesFromBrowser string
}

func NewClient(binaryPath string) *Client {
	return NewClientWithOptions(ClientOptions{BinaryPath: binaryPath})
}

type ClientOptions struct {
	BinaryPath         string
	CookiesFile        string
	CookiesFromBrowser string
}

func NewClientWithOptions(opts ClientOptions) *Client {
	if opts.BinaryPath == "" {
		opts.BinaryPath = "yt-dlp"
	}
	return &Client{
		BinaryPath:         opts.BinaryPath,
		CookiesFile:        strings.TrimSpace(opts.CookiesFile),
		CookiesFromBrowser: strings.TrimSpace(opts.CookiesFromBrowser),
	}
}

func (c *Client) CookiesConfigured() bool {
	if c == nil {
		return false
	}
	if c.CookiesFile != "" {
		if _, err := os.Stat(c.CookiesFile); err == nil {
			return true
		}
	}
	return c.CookiesFromBrowser != ""
}

func (c *Client) cookieArgs() ([]string, error) {
	if c == nil {
		return nil, nil
	}
	if c.CookiesFile != "" {
		if _, err := os.Stat(c.CookiesFile); err != nil {
			return nil, fmt.Errorf("cookies file not found: %s", c.CookiesFile)
		}
		return []string{"--cookies", c.CookiesFile}, nil
	}
	if c.CookiesFromBrowser != "" {
		return []string{"--cookies-from-browser", c.CookiesFromBrowser}, nil
	}
	return nil, nil
}

func (c *Client) appendCookieArgs(args []string) ([]string, error) {
	cookieArgs, err := c.cookieArgs()
	if err != nil {
		return nil, err
	}
	if len(cookieArgs) == 0 {
		return args, nil
	}
	return append(append([]string{}, args[:len(args)-1]...), append(cookieArgs, args[len(args)-1])...), nil
}

func (c *Client) Available() bool {
	if c == nil || c.BinaryPath == "" {
		return false
	}
	if filepath.IsAbs(c.BinaryPath) {
		if _, err := os.Stat(c.BinaryPath); err == nil {
			return true
		}
	}
	_, err := exec.LookPath(c.BinaryPath)
	return err == nil
}

type ytdlpInfo struct {
	Type       string  `json:"_type"`
	Title      string  `json:"title"`
	Duration   float64 `json:"duration"`
	Thumbnail  string  `json:"thumbnail"`
	Uploader   string  `json:"uploader"`
	Extractor  string  `json:"extractor"`
	Ext        string  `json:"ext"`
	Formats    []Format `json:"formats"`
}

func (c *Client) Probe(ctx context.Context, url string) (*ProbeResult, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	cmd := exec.CommandContext(ctx, c.BinaryPath,
		"--dump-single-json",
		"--no-download",
		"--no-playlist",
		"--no-warnings",
		url,
	)
	args, err := c.appendCookieArgs(cmd.Args)
	if err != nil {
		return nil, err
	}
	cmd.Args = args
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := compactOutput(stderr.Bytes())
		if msg == "" {
			msg = compactOutput(stdout.Bytes())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("yt-dlp probe failed: %s", msg)
	}

	var info ytdlpInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("parse yt-dlp output: %w", err)
	}
	if info.Type == "playlist" {
		return nil, fmt.Errorf("playlists are not supported; paste a single video or audio URL")
	}
	if strings.TrimSpace(info.Title) == "" {
		return nil, fmt.Errorf("could not extract metadata from URL")
	}

	result := &ProbeResult{
		Title:      info.Title,
		Duration:   info.Duration,
		Thumbnail:  info.Thumbnail,
		Uploader:   info.Uploader,
		Extractor:  info.Extractor,
		Formats:    filterFormats(info.Formats),
		Suggested:  buildPresets(info.Formats),
		DefaultExt: info.Ext,
	}
	return result, nil
}

func filterFormats(formats []Format) []Format {
	var out []Format
	for _, f := range formats {
		id := strings.TrimSpace(f.ID)
		if id == "" {
			continue
		}
		f.ID = id
		out = append(out, f)
	}
	return out
}

func buildPresets(formats []Format) []Preset {
	presets := []Preset{
		{Label: "Best available", FormatID: "bestvideo+bestaudio/best", Group: "best"},
		{Label: "Best video + audio (merge)", FormatID: "bv*+ba/b", Group: "best"},
		{Label: "1080p or best", FormatID: "bestvideo[height<=1080]+bestaudio/best[height<=1080]", Group: "video"},
		{Label: "720p or best", FormatID: "bestvideo[height<=720]+bestaudio/best[height<=720]", Group: "video"},
		{Label: "480p or best", FormatID: "bestvideo[height<=480]+bestaudio/best[height<=480]", Group: "video"},
		{Label: "Audio only (best)", FormatID: "ba/b", Group: "audio"},
	}

	hasAudioOnly := false
	for _, f := range formats {
		if f.VCodec == "none" || strings.EqualFold(f.VCodec, "audio only") {
			hasAudioOnly = true
			break
		}
	}
	if !hasAudioOnly {
		_ = hasAudioOnly
	}
	return presets
}

var (
	invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	whitespaceRun        = regexp.MustCompile(`\s+`)
)

func SanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = invalidFilenameChars.ReplaceAllString(name, "")
	name = whitespaceRun.ReplaceAllString(name, " ")
	name = strings.Trim(name, ". ")
	if name == "" {
		return "download"
	}
	if len(name) > 200 {
		name = strings.TrimSpace(name[:200])
	}
	return name
}

func (c *Client) Download(ctx context.Context, opts DownloadOptions) (string, error) {
	opts.URL = strings.TrimSpace(opts.URL)
	opts.FormatID = strings.TrimSpace(opts.FormatID)
	opts.OutputDir = filepath.Clean(strings.TrimSpace(opts.OutputDir))
	opts.Filename = SanitizeFilename(opts.Filename)

	if opts.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	if opts.FormatID == "" {
		opts.FormatID = "bestvideo+bestaudio/best"
	}
	if opts.OutputDir == "" {
		return "", fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	outputTemplate := filepath.Join(opts.OutputDir, opts.Filename+".%(ext)s")

	cmd := exec.CommandContext(ctx, c.BinaryPath,
		"--no-playlist",
		"--no-warnings",
		"--newline",
		"-f", opts.FormatID,
		"-o", outputTemplate,
		"--print", "after_move:filepath",
		opts.URL,
	)
	args, err := c.appendCookieArgs(cmd.Args)
	if err != nil {
		return "", err
	}
	cmd.Args = args
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start yt-dlp: %w", err)
	}

	progressCh := make(chan struct{})
	go func() {
		defer close(progressCh)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if opts.ProgressFn != nil {
				if pct := parseProgress(sc.Text()); pct >= 0 {
					opts.ProgressFn(pct)
				}
			}
		}
	}()

	var outBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, stdout)

	waitErr := cmd.Wait()
	<-progressCh

	finalPath := ""
	lines := strings.Split(strings.TrimSpace(outBuf.String()), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			finalPath = line
			break
		}
	}

	if waitErr != nil {
		if finalPath != "" {
			if _, statErr := os.Stat(finalPath); statErr == nil {
				return finalPath, nil
			}
		}
		return "", fmt.Errorf("yt-dlp download failed: %v", waitErr)
	}
	if finalPath == "" {
		var err error
		finalPath, err = findNewestFile(opts.OutputDir, opts.Filename)
		if err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(finalPath); err != nil {
		return "", fmt.Errorf("downloaded file not found: %w", err)
	}
	if opts.ProgressFn != nil {
		opts.ProgressFn(100)
	}
	return filepath.Clean(finalPath), nil
}

var progressRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)

func parseProgress(line string) float64 {
	m := progressRe.FindStringSubmatch(line)
	if len(m) < 2 {
		return -1
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return -1
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func findNewestFile(dir, prefix string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var bestPath string
	var bestTime int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UnixNano() > bestTime {
			bestTime = info.ModTime().UnixNano()
			bestPath = filepath.Join(dir, name)
		}
	}
	if bestPath == "" {
		return "", fmt.Errorf("no downloaded file found in %s", dir)
	}
	return bestPath, nil
}

func compactOutput(out []byte) string {
	s := strings.Join(strings.Fields(strings.TrimSpace(string(out))), " ")
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}
