package db

import (
	"strings"
	"time"
)

type EncodeSettings struct {
	VideoCodec           string `json:"video_codec"`
	AudioCodec           string `json:"audio_codec"`
	AudioBitrate         string `json:"audio_bitrate"`
	CRF                  string `json:"crf"`
	Preset               string `json:"preset"`
	Tune                 string `json:"tune"`
	Profile              string `json:"profile"`
	Container            string `json:"container"`
	OutputDir            string `json:"output_dir"`
	DeleteSource         bool   `json:"delete_source"`
	KeepOriginalIfLarger bool   `json:"keep_original_if_larger"`
	KeepExtraStreams     bool   `json:"keep_extra_streams"`
}

func (e *EncodeSettings) ApplyDefaults() {
	e.ApplyDefaultsFor("video")
}

func (e *EncodeSettings) ApplyDefaultsFor(mediaType string) {
	if mediaType == "audio" {
		e.applyAudioDefaults()
		return
	}
	if e.VideoCodec == "" {
		e.VideoCodec = "libx265"
	}
	if e.AudioCodec == "" {
		e.AudioCodec = "copy"
	}
	if e.Container == "" {
		e.Container = "mkv"
	}
}

func (e *EncodeSettings) applyAudioDefaults() {
	codec := strings.ToLower(strings.TrimSpace(e.AudioCodec))
	container := strings.ToLower(strings.TrimSpace(e.Container))
	if codec == "" || (codec == "copy" && (container == "mkv" || container == "mp4")) {
		codec = "libmp3lame"
		container = "mp3"
	}
	e.AudioCodec = codec
	if container == "" {
		container = AudioContainerForCodec(codec)
	}
	e.Container = container
	if e.VideoCodec == "" {
		e.VideoCodec = "libx265"
	}
	if e.AudioBitrate == "" && AudioCodecNeedsBitrate(codec) {
		e.AudioBitrate = DefaultAudioBitrate(codec)
	}
}

func AudioContainerForCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "libmp3lame", "mp3":
		return "mp3"
	case "aac":
		return "m4a"
	case "libopus", "opus":
		return "opus"
	case "libvorbis", "vorbis":
		return "ogg"
	case "flac":
		return "flac"
	case "pcm_s16le", "wav":
		return "wav"
	default:
		return ""
	}
}

func AudioCodecNeedsBitrate(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "flac", "pcm_s16le", "wav", "copy", "":
		return false
	default:
		return true
	}
}

func DefaultAudioBitrate(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "libmp3lame", "mp3":
		return "320k"
	case "aac":
		return "192k"
	case "libopus", "opus":
		return "128k"
	case "libvorbis", "vorbis":
		return "192k"
	default:
		return "192k"
	}
}

func (e EncodeSettings) OutputExt(mediaType string) string {
	if mediaType == "audio" {
		c := strings.ToLower(strings.TrimSpace(e.Container))
		if c == "" {
			c = AudioContainerForCodec(e.AudioCodec)
		}
		if c == "" {
			return ""
		}
		return "." + c
	}
	if e.Container == "mp4" {
		return ".mp4"
	}
	return ".mkv"
}

type WatchFolder struct {
	ID                int    `json:"id"`
	FolderPath        string `json:"folder_path"`
	MediaType         string `json:"media_type"`
	TargetResolution  string `json:"target_resolution"`
	CustomFFmpegFlags string `json:"custom_ffmpeg_flags"`
	Enabled           bool   `json:"enabled"`
	EncodeSettings
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (f WatchFolder) NewJob(filePath string, originalSize int64, priority int) Job {
	f.EncodeSettings.ApplyDefaultsFor(f.MediaType)
	return Job{
		FilePath:         filePath,
		MediaType:        f.MediaType,
		Priority:         priority,
		OriginalSize:     originalSize,
		TargetResolution: f.TargetResolution,
		FFmpegFlags:      f.CustomFFmpegFlags,
		EncodeSettings:   f.EncodeSettings,
	}
}

type Job struct {
	ID               int    `json:"id"`
	FilePath         string `json:"file_path"`
	MediaType        string `json:"media_type"`
	Status           string `json:"status"` // pending, processing, failed
	Priority         int    `json:"priority"`
	OriginalSize     int64  `json:"original_size"`
	TargetResolution string `json:"target_resolution"`
	FFmpegFlags      string `json:"ffmpeg_flags"`
	ErrorMessage     string `json:"error_message"`
	EncodeSettings
	Force         bool      `json:"force"`
	FFmpegCommand string    `json:"ffmpeg_command,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type JobReport struct {
	ID               int     `json:"id"`
	FilePath         string  `json:"file_path"`
	MediaType        string  `json:"media_type"`
	Status           string  `json:"status"` // success, failed, skipped
	OriginalSize     int64   `json:"original_size"`
	EncodedSize      int64   `json:"encoded_size"`
	SizeSaved        int64   `json:"size_saved"`
	ProcessingTime   float64 `json:"processing_time"`
	TargetResolution string  `json:"target_resolution"`
	FFmpegFlags      string  `json:"ffmpeg_flags"`
	ErrorMessage     string  `json:"error_message"`
	FFmpegCommand    string  `json:"ffmpeg_command,omitempty"`
	EncodeSettings
	CreatedAt time.Time `json:"created_at"`
}

type DownloadJob struct {
	ID            int       `json:"id"`
	URL           string    `json:"url"`
	Title         string    `json:"title"`
	Filename      string    `json:"filename"`
	DestPath      string    `json:"dest_path"`
	FilePath      string    `json:"file_path"`
	FormatID      string    `json:"format_id"`
	Mode          string    `json:"mode"` // encode, download_only
	WatchFolderID *int      `json:"watch_folder_id,omitempty"`
	Status        string    `json:"status"`
	Progress      float64   `json:"progress"`
	ErrorMessage  string    `json:"error_message"`
	FileSize      int64     `json:"file_size"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
