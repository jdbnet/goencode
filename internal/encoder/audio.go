package encoder

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func (m *FFmpegManager) BuildAudioCmd(inputPath, outputPath, ffmpegFlags string) (*exec.Cmd, error) {
	return exec.Command(m.BinaryPath, buildAudioArgs(inputPath, outputPath, ffmpegFlags)...), nil
}

func buildAudioArgs(inputPath, outputPath, ffmpegFlags string) []string {
	args := []string{"-i", inputPath, "-c:a", "libmp3lame", "-b:a", "320k"}
	if ffmpegFlags != "" {
		args = append(args, ShlexSplit(ffmpegFlags)...)
	}
	return append(args, outputPath, "-y")
}

func CheckAudioSkip(inputPath string) (bool, string) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", inputPath)
	out, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	codec := strings.TrimSpace(string(out))

	if codec == "mp3" {
		cmdBr := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "a:0",
			"-show_entries", "stream=bit_rate", "-of", "csv=p=0", inputPath)
		outBr, err := cmdBr.Output()
		if err == nil {
			br, _ := strconv.Atoi(strings.TrimSpace(string(outBr)))
			if br >= 300000 {
				return true, fmt.Sprintf("Already MP3 at %dkbps (target: 320kbps)", br/1000)
			}
		}
	}
	return false, ""
}

func (m *FFmpegManager) ShouldSkipAudio(inputPath string) bool {
	skip, _ := CheckAudioSkip(inputPath)
	return skip
}
