package encoder

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// FFmpegManager handles execution and probing
type FFmpegManager struct {
	BinaryPath string
}

func NewManager(binaryPath string) *FFmpegManager {
	if binaryPath == "" {
		binaryPath = "ffmpeg"
	}
	return &FFmpegManager{BinaryPath: binaryPath}
}

func (m *FFmpegManager) probeBin() string {
	return ffprobePath(m.BinaryPath)
}

func ffprobePath(ffmpegPath string) string {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	base := filepath.Base(ffmpegPath)
	name := "ffprobe"
	if strings.HasSuffix(strings.ToLower(base), ".exe") {
		name = "ffprobe.exe"
	}
	dir := filepath.Dir(ffmpegPath)
	if dir == "." {
		return name
	}
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return name
}

func runFfprobe(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := compactOutput(stderr.Bytes()); msg != "" {
			return stdout.Bytes(), fmt.Errorf("ffprobe error: %w (%s)", err, msg)
		}
		return stdout.Bytes(), fmt.Errorf("ffprobe error: %w", err)
	}
	return stdout.Bytes(), nil
}

func compactOutput(out []byte) string {
	s := strings.Join(strings.Fields(strings.TrimSpace(string(out))), " ")
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}

func firstCSVField(out []byte) string {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.EqualFold(line, "n/a") {
			continue
		}
		if i := strings.IndexByte(line, ','); i > 0 {
			line = strings.TrimSpace(line[:i])
		}
		return line
	}
	return ""
}

func parseWH(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	sep := ","
	if strings.Contains(s, "x") && !strings.Contains(s, ",") {
		sep = "x"
	}
	var nums []int
	for _, p := range strings.Split(s, sep) {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 {
			continue
		}
		nums = append(nums, n)
	}
	if len(nums) < 2 {
		return 0, 0, false
	}
	return nums[0], nums[1], true
}

func parseResolution(out string) (int, int, error) {
	var nums []int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		if w, h, ok := parseWH(line); ok {
			return w, h, nil
		}
		if n, err := strconv.Atoi(line); err == nil && n > 0 {
			nums = append(nums, n)
			if len(nums) >= 2 {
				return nums[0], nums[1], nil
			}
		}
	}
	got := compactOutput([]byte(out))
	if got == "" {
		return 0, 0, fmt.Errorf("invalid resolution format (empty ffprobe output; is ffprobe installed and is this a video file?)")
	}
	return 0, 0, fmt.Errorf("invalid resolution format %q", got)
}

func (m *FFmpegManager) ProbeCodec(filePath, streamType string) (string, error) {
	out, err := runFfprobe(m.probeBin(), "-v", "error", "-select_streams", streamType+":0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", filePath)
	if err != nil {
		return "", err
	}
	return firstCSVField(out), nil
}

func (m *FFmpegManager) ProbeResolution(filePath string) (int, int, error) {
	out, err := runFfprobe(m.probeBin(), "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", filePath)
	if err != nil {
		return 0, 0, err
	}
	return parseResolution(string(out))
}

func (m *FFmpegManager) ProbeDuration(filePath string) (float64, error) {
	out, err := runFfprobe(m.probeBin(), "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", filePath)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(firstCSVField(out), 64)
}

func (m *FFmpegManager) ValidateFile(filePath string) error {
	_, err := runFfprobe(m.probeBin(), "-v", "error", "-show_format", "-show_streams", filePath)
	if err != nil {
		return fmt.Errorf("file validation failed: %w", err)
	}
	return nil
}

// ParseProgress is a utility to parse time=XX:XX:XX.XX from stderr line to calculate percentage if duration is known.
// Will be used by the queue worker reading from a pipe.
func ParseProgress(line string, durationSec float64) float64 {
	// Example line: frame=  123 fps= 30 q=22.0 size= 2048kB time=00:00:10.50 bitrate=1500.0kbits/s speed=1.5x
	idx := strings.Index(line, "time=")
	if idx == -1 {
		return -1
	}
	timeStr := line[idx+5:]
	spaceIdx := strings.Index(timeStr, " ")
	if spaceIdx != -1 {
		timeStr = timeStr[:spaceIdx]
	}

	// timeStr is HH:MM:SS.ms
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return -1
	}

	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	s, _ := strconv.ParseFloat(parts[2], 64)

	currentSec := h*3600 + m*60 + s
	if durationSec > 0 {
		return (currentSec / durationSec) * 100.0
	}
	return -1
}

// ShlexSplit splits a command string like a shell would.
// A simple implementation since go doesn't have shlex.split built-in.
func ShlexSplit(s string) []string {
	var args []string
	var buf bytes.Buffer
	inQuotes := false
	var quoteChar rune

	for _, r := range s {
		if inQuotes {
			if r == quoteChar {
				inQuotes = false
			} else {
				buf.WriteRune(r)
			}
		} else {
			if r == '\'' || r == '"' {
				inQuotes = true
				quoteChar = r
			} else if r == ' ' {
				if buf.Len() > 0 {
					args = append(args, buf.String())
					buf.Reset()
				}
			} else {
				buf.WriteRune(r)
			}
		}
	}
	if buf.Len() > 0 {
		args = append(args, buf.String())
	}
	return args
}

func FormatCommand(cmd *exec.Cmd) string {
	if cmd == nil {
		return ""
	}
	if len(cmd.Args) > 0 {
		return FormatArgs(cmd.Args)
	}
	if cmd.Path != "" {
		return quoteArg(cmd.Path)
	}
	return ""
}

func FormatArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = quoteArg(a)
	}
	return strings.Join(quoted, " ")
}

func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	for _, r := range s {
		switch r {
		case ' ', '\t', '"', '\'', '\\', '$', '`', '|', '&', ';', '<', '>', '(', ')', '{', '}', '*', '?', '[', ']', '#', '~':
			return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
		}
	}
	return s
}

func (m *FFmpegManager) PreviewCommand(mediaType, inputPath, outputPath string, opt VideoEncodeOptions) string {
	if m == nil {
		return ""
	}
	var cmd *exec.Cmd
	if mediaType == "audio" {
		cmd, _ = m.BuildAudioCmd(inputPath, outputPath, AudioEncodeOptions{
			Codec:       opt.AudioCodec,
			Bitrate:     opt.AudioBitrate,
			Container:   opt.Container,
			CustomFlags: opt.CustomFlags,
		})
	} else {
		cmd, _ = m.BuildVideoCmd(inputPath, outputPath, opt)
	}
	return FormatCommand(cmd)
}
