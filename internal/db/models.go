package db

import (
	"time"
)

type EncodeSettings struct {
	VideoCodec           string `json:"video_codec"`
	AudioCodec           string `json:"audio_codec"`
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

func (e EncodeSettings) OutputExt(mediaType string) string {
	if mediaType == "audio" {
		return ".mp3"
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
	f.EncodeSettings.ApplyDefaults()
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
