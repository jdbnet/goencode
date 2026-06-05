package encoder

import (
	"os/exec"
	"strconv"
	"strings"
)

func (m *FFmpegManager) BuildAudioCmd(inputPath, outputPath, ffmpegFlags string) (*exec.Cmd, error) {
	args := []string{"-i", inputPath, "-c:a", "libmp3lame", "-b:a", "320k"}

	if ffmpegFlags != "" {
		userArgs := ShlexSplit(ffmpegFlags)
		args = append(args, userArgs...)
	}

	args = append(args, outputPath, "-y")

	return exec.Command(m.BinaryPath, args...), nil
}

func (m *FFmpegManager) ShouldSkipAudio(inputPath string) bool {
	// Probe the codec
	cmd := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", inputPath)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	codec := strings.TrimSpace(string(out))

	// If it's already mp3, check bitrate
	if codec == "mp3" {
		cmdBr := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "a:0",
			"-show_entries", "stream=bit_rate", "-of", "csv=p=0", inputPath)
		outBr, err := cmdBr.Output()
		if err == nil {
			br, _ := strconv.Atoi(strings.TrimSpace(string(outBr)))
			if br >= 300000 {
				return true // It's ~320k mp3
			}
		}
	}
	return false
}
