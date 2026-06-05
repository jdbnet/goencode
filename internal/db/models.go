package db

import (
	"time"
)

type WatchFolder struct {
	ID                int       `json:"id"`
	FolderPath        string    `json:"folder_path"`
	MediaType         string    `json:"media_type"`
	TargetResolution  string    `json:"target_resolution"`
	CustomFFmpegFlags string    `json:"custom_ffmpeg_flags"`
	Enabled           bool      `json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Job struct {
	ID               int       `json:"id"`
	FilePath         string    `json:"file_path"`
	MediaType        string    `json:"media_type"`
	Status           string    `json:"status"` // pending, processing, failed
	Priority         int       `json:"priority"`
	OriginalSize     int64     `json:"original_size"`
	TargetResolution string    `json:"target_resolution"`
	FFmpegFlags      string    `json:"ffmpeg_flags"`
	ErrorMessage     string    `json:"error_message"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type JobReport struct {
	ID               int       `json:"id"`
	FilePath         string    `json:"file_path"`
	MediaType        string    `json:"media_type"`
	Status           string    `json:"status"` // success, failed, skipped
	OriginalSize     int64     `json:"original_size"`
	EncodedSize      int64     `json:"encoded_size"`
	SizeSaved        int64     `json:"size_saved"`
	ProcessingTime   float64   `json:"processing_time"`
	TargetResolution string    `json:"target_resolution"`
	FFmpegFlags      string    `json:"ffmpeg_flags"`
	ErrorMessage     string    `json:"error_message"`
	CreatedAt        time.Time `json:"created_at"`
}
