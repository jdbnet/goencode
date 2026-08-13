package encoder

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type AudioEncodeOptions struct {
	Codec, Bitrate, Container, CustomFlags string
	Threads                                int
}

func (m *FFmpegManager) BuildAudioCmd(inputPath, outputPath string, opt AudioEncodeOptions) (*exec.Cmd, error) {
	if m != nil && opt.Threads == 0 {
		opt.Threads = m.Threads
	}
	return exec.Command(m.BinaryPath, buildAudioArgs(inputPath, outputPath, opt)...), nil
}

func buildAudioArgs(inputPath, outputPath string, opt AudioEncodeOptions) []string {
	opt = normalizeAudioOptions(opt)
	args := []string{"-i", inputPath}
	if !hasMapFlag(opt.CustomFlags) {
		args = append(args, "-vn", "-map", "0:a:0?")
	}

	switch opt.Codec {
	case "copy":
		args = append(args, "-c:a", "copy")
	case "flac":
		args = append(args, "-c:a", "flac")
	case "pcm_s16le":
		args = append(args, "-c:a", "pcm_s16le")
	default:
		args = append(args, "-c:a", opt.Codec)
		if br := NormalizeAudioBitrate(opt.Bitrate); br != "" {
			args = append(args, "-b:a", br)
		}
	}

	if f := audioMuxer(opt.Container); f != "" {
		args = append(args, "-f", f)
	}

	args = append(args, threadFlags(opt.Codec, opt.Threads, opt.CustomFlags)...)

	if opt.CustomFlags != "" {
		args = append(args, ShlexSplit(opt.CustomFlags)...)
	}
	return append(args, outputPath, "-y")
}

func normalizeAudioOptions(opt AudioEncodeOptions) AudioEncodeOptions {
	codec := strings.ToLower(strings.TrimSpace(opt.Codec))
	switch codec {
	case "", "mp3":
		codec = "libmp3lame"
	case "opus":
		codec = "libopus"
	case "vorbis":
		codec = "libvorbis"
	case "wav":
		codec = "pcm_s16le"
	}
	opt.Codec = codec
	if opt.Container == "" {
		opt.Container = audioContainer(codec)
	}
	if opt.Bitrate == "" && audioNeedsBitrate(codec) {
		opt.Bitrate = defaultAudioBitrate(codec)
	}
	return opt
}

func audioContainer(codec string) string {
	switch codec {
	case "libmp3lame":
		return "mp3"
	case "aac":
		return "m4a"
	case "libopus":
		return "opus"
	case "libvorbis":
		return "ogg"
	case "flac":
		return "flac"
	case "pcm_s16le":
		return "wav"
	default:
		return ""
	}
}

func audioMuxer(container string) string {
	switch strings.ToLower(strings.TrimSpace(container)) {
	case "mp3":
		return "mp3"
	case "m4a", "mp4":
		return "ipod"
	case "opus":
		return "opus"
	case "ogg":
		return "ogg"
	case "flac":
		return "flac"
	case "wav":
		return "wav"
	default:
		return ""
	}
}

func audioNeedsBitrate(codec string) bool {
	switch codec {
	case "flac", "pcm_s16le", "copy":
		return false
	default:
		return true
	}
}

func defaultAudioBitrate(codec string) string {
	switch codec {
	case "libmp3lame":
		return "320k"
	case "libopus":
		return "128k"
	default:
		return "192k"
	}
}

func NormalizeAudioBitrate(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, "bps")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasSuffix(s, "k") || strings.HasSuffix(s, "m") {
		return s
	}
	if _, err := strconv.Atoi(s); err == nil {
		return s + "k"
	}
	return s
}

func audioBitrateBps(s string) int {
	s = NormalizeAudioBitrate(s)
	if s == "" {
		return 0
	}
	mult := 1
	switch {
	case strings.HasSuffix(s, "k"):
		mult = 1000
		s = strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult = 1000000
		s = strings.TrimSuffix(s, "m")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n * mult
}

func probeAudioCodecName(ffmpegCodec string) string {
	switch strings.ToLower(strings.TrimSpace(ffmpegCodec)) {
	case "libmp3lame", "mp3":
		return "mp3"
	case "aac":
		return "aac"
	case "libopus", "opus":
		return "opus"
	case "libvorbis", "vorbis":
		return "vorbis"
	case "flac":
		return "flac"
	case "pcm_s16le", "wav":
		return "pcm"
	default:
		return ""
	}
}

func CheckAudioSkip(inputPath, ffmpegCodec, bitrate string) (bool, string) {
	opt := normalizeAudioOptions(AudioEncodeOptions{Codec: ffmpegCodec, Bitrate: bitrate})
	if opt.Codec == "copy" {
		return false, ""
	}

	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", inputPath)
	out, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	probed := firstCSVField(out)
	br := 0
	cmdBr := exec.Command("ffprobe", "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=bit_rate", "-of", "csv=p=0", inputPath)
	if outBr, err := cmdBr.Output(); err == nil {
		br, _ = strconv.Atoi(firstCSVField(outBr))
	}
	return shouldSkipAudio(probed, br, opt.Codec, opt.Bitrate)
}

func shouldSkipAudio(probedCodec string, probedBps int, ffmpegCodec, bitrate string) (bool, string) {
	opt := normalizeAudioOptions(AudioEncodeOptions{Codec: ffmpegCodec, Bitrate: bitrate})
	if opt.Codec == "copy" {
		return false, ""
	}
	target := probeAudioCodecName(opt.Codec)
	if target == "" {
		return false, ""
	}
	if target == "pcm" {
		if !strings.HasPrefix(probedCodec, "pcm_") {
			return false, ""
		}
		return true, "Already WAV/PCM"
	}
	if probedCodec != target {
		return false, ""
	}

	label := strings.ToUpper(target)
	if !audioNeedsBitrate(opt.Codec) {
		return true, fmt.Sprintf("Already %s", label)
	}

	targetBps := audioBitrateBps(opt.Bitrate)
	if targetBps <= 0 {
		return true, fmt.Sprintf("Already %s", label)
	}
	if probedBps <= 0 {
		return false, ""
	}
	if probedBps+20000 >= targetBps {
		return true, fmt.Sprintf("Already %s at %dkbps (target: %s)", label, probedBps/1000, NormalizeAudioBitrate(opt.Bitrate))
	}
	return false, ""
}

func (m *FFmpegManager) ShouldSkipAudio(inputPath string) bool {
	skip, _ := CheckAudioSkip(inputPath, "libmp3lame", "320k")
	return skip
}
