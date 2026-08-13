package encoder

import (
	"bytes"
	"fmt"
	"os/exec"
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

func (m *FFmpegManager) ProbeCodec(filePath, streamType string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-select_streams", streamType+":0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", filePath)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe error: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *FFmpegManager) ProbeResolution(filePath string) (int, int, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", filePath)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe error: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid resolution format")
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("invalid resolution numbers")
	}
	return w, h, nil
}

func (m *FFmpegManager) ProbeDuration(filePath string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-show_entries", "format=duration", "-of", "csv=p=0", filePath)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe error: %w", err)
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

func (m *FFmpegManager) ValidateFile(filePath string) error {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-show_format", "-show_streams", filePath)
	if err := cmd.Run(); err != nil {
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
