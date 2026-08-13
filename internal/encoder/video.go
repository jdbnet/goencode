package encoder

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type VideoEncodeOptions struct {
	TargetResolution              string
	VideoCodec, AudioCodec        string
	CRF, Preset, Tune, Profile    string
	Container, CustomFlags        string
	OriginalWidth, OriginalHeight int
}

func (m *FFmpegManager) BuildVideoCmd(inputPath, outputPath string, opt VideoEncodeOptions) (*exec.Cmd, error) {
	args := buildVideoArgs(inputPath, outputPath, opt)
	return exec.Command(m.BinaryPath, args...), nil
}

func buildVideoArgs(inputPath, outputPath string, opt VideoEncodeOptions) []string {
	args := []string{"-i", inputPath}

	codec := opt.VideoCodec
	if codec == "" {
		codec = "libx265"
	}

	if codec != "copy" && opt.TargetResolution != "" {
		targetHeightStr := strings.TrimRight(strings.ToLower(opt.TargetResolution), "p")
		targetHeight, err := strconv.Atoi(targetHeightStr)
		if err == nil && targetHeight > 0 && opt.OriginalHeight > targetHeight {
			scaleWidth := int((float64(opt.OriginalWidth) / float64(opt.OriginalHeight)) * float64(targetHeight))
			if scaleWidth%2 != 0 {
				scaleWidth--
			}
			args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", scaleWidth, targetHeight))
		}
	}

	args = append(args, "-c:v", codec)

	switch opt.AudioCodec {
	case "aac":
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	case "ac3":
		args = append(args, "-c:a", "ac3", "-b:a", "384k")
	default:
		args = append(args, "-c:a", "copy")
	}

	if codec != "copy" {
		preset := opt.Preset
		if preset == "" && codec != "libsvtav1" {
			preset = "medium"
		}
		if preset != "" {
			args = append(args, "-preset", preset)
		}

		crf := opt.CRF
		if crf == "" {
			if codec == "libsvtav1" {
				crf = "35"
			} else {
				crf = "22"
			}
		}
		args = append(args, "-crf", crf)

		if opt.Tune != "" {
			args = append(args, "-tune", opt.Tune)
		}
		if opt.Profile != "" {
			args = append(args, "-profile:v", opt.Profile)
		}
	}

	container := opt.Container
	if container == "" {
		container = "mkv"
	}
	if container == "mp4" {
		args = append(args, "-f", "mp4")
		if codec == "libx265" {
			args = append(args, "-tag:v", "hvc1")
		} else if codec == "libsvtav1" {
			args = append(args, "-tag:v", "av01")
		}
	} else {
		args = append(args, "-f", "matroska")
	}

	if opt.CustomFlags != "" {
		args = append(args, ShlexSplit(opt.CustomFlags)...)
	}

	args = append(args, outputPath, "-y")
	return args
}

func ProbeCodecName(videoCodec string) string {
	switch videoCodec {
	case "libx264":
		return "h264"
	case "libsvtav1":
		return "av1"
	case "copy":
		return ""
	default:
		return "hevc"
	}
}

func codecLabel(probed string) string {
	switch probed {
	case "h264":
		return "H.264"
	case "av1":
		return "AV1"
	default:
		return "HEVC"
	}
}

func CheckVideoSkip(inputPath, targetResolution, videoCodec string) (bool, string) {
	if videoCodec == "copy" {
		return false, ""
	}

	cmd := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", inputPath)
	out, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	codec := strings.TrimSpace(string(out))

	cmdHeight := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "v:0",
		"-show_entries", "stream=height", "-of", "csv=p=0", inputPath)
	outHeight, err := cmdHeight.Output()
	if err != nil {
		return false, ""
	}

	return shouldSkipVideo(codec, strings.TrimSpace(string(outHeight)), targetResolution, videoCodec)
}

func shouldSkipVideo(probedCodec, probedHeight, targetResolution, videoCodec string) (bool, string) {
	if videoCodec == "copy" {
		return false, ""
	}
	target := ProbeCodecName(videoCodec)
	if target == "" || probedCodec != target {
		return false, ""
	}

	label := codecLabel(target)
	if targetResolution == "" {
		return true, fmt.Sprintf("Already %s - no re-encode needed", label)
	}

	currentHeight, _ := strconv.Atoi(strings.TrimSpace(probedHeight))
	targetHeight, _ := strconv.Atoi(strings.TrimRight(strings.ToLower(targetResolution), "p"))
	if currentHeight > 0 && targetHeight > 0 && currentHeight <= targetHeight {
		return true, fmt.Sprintf("Already %s at %dp (target: %s)", label, currentHeight, targetResolution)
	}

	return false, ""
}

func (m *FFmpegManager) ShouldSkipVideo(inputPath, targetResolution, videoCodec string) bool {
	skip, _ := CheckVideoSkip(inputPath, targetResolution, videoCodec)
	return skip
}
