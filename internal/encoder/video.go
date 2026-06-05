package encoder

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func (m *FFmpegManager) BuildVideoCmd(inputPath, outputPath, targetResolution, ffmpegFlags string, originalWidth, originalHeight int) (*exec.Cmd, error) {
	args := []string{"-i", inputPath}

	if targetResolution != "" {
		targetHeightStr := strings.TrimRight(strings.ToLower(targetResolution), "p")
		targetHeight, err := strconv.Atoi(targetHeightStr)
		if err == nil && targetHeight > 0 && originalHeight > targetHeight {
			scaleWidth := int((float64(originalWidth) / float64(originalHeight)) * float64(targetHeight))
			if scaleWidth%2 != 0 {
				scaleWidth--
			}
			args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", scaleWidth, targetHeight))
		}
	}

	args = append(args, "-c:v", "libx265", "-c:a", "copy", "-preset", "medium", "-crf", "22", "-f", "matroska")

	if ffmpegFlags != "" {
		userArgs := ShlexSplit(ffmpegFlags)
		args = append(args, userArgs...)
	}

	args = append(args, outputPath, "-y")

	return exec.Command(m.BinaryPath, args...), nil
}

func (m *FFmpegManager) ShouldSkipVideo(inputPath, targetResolution string) bool {
	// Probe the codec
	cmd := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", inputPath)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	codec := strings.TrimSpace(string(out))

	// Probe the height
	cmdHeight := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "v:0",
		"-show_entries", "stream=height", "-of", "csv=p=0", inputPath)
	outHeight, err := cmdHeight.Output()
	if err != nil {
		return false
	}
	
	// If it's not H.264 or H.265, do not skip (encode it)
	if codec != "h264" && codec != "hevc" {
		return false
	}
	
	// If it's H.265 and no target resolution is set, it's already in the target codec
	if codec == "hevc" && targetResolution == "" {
		return true
	}
	
	// If it's H.265 and target resolution is set, only skip if current height is <= target height
	if codec == "hevc" && targetResolution != "" {
		currentHeight, _ := strconv.Atoi(strings.TrimSpace(string(outHeight)))
		targetHeight, _ := strconv.Atoi(strings.TrimRight(strings.ToLower(targetResolution), "p"))
		if currentHeight > 0 && targetHeight > 0 && currentHeight <= targetHeight {
			return true
		}
	}
	
	return false
}
