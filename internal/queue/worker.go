package queue

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"goencode/internal/db"
	"goencode/internal/encoder"
	"io"
)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}

func (m *Manager) workerLoop() {
	for {
		select {
		case <-m.StopChan:
			return
		case <-m.TriggerChan:
			m.processNextJob()
		}
	}
}

func (m *Manager) processNextJob() {
	m.mu.Lock()
	if m.isProcessing {
		m.mu.Unlock()
		return
	}
	m.isProcessing = true
	m.mu.Unlock()

	var hasJobs bool
	defer func() {
		m.mu.Lock()
		m.isProcessing = false
		m.mu.Unlock()
		// Only trigger immediately if we know there might be more jobs
		if hasJobs {
			m.Trigger()
		}
	}()

	jobs, err := db.GetPendingJobs()
	if err != nil {
		log.Printf("Failed to get pending jobs: %v", err)
		return
	}

	if len(jobs) == 0 {
		return
	}
	hasJobs = true

	job := jobs[0]
	log.Printf("Starting job %d for %s", job.ID, job.FilePath)
	
	err = db.UpdateJobStatus(job.ID, "processing", "")
	if err != nil {
		log.Printf("Failed to update job status: %v", err)
		return
	}

	job.Status = "processing"
	job.UpdatedAt = time.Now()
	m.NotifySSE("job_started", job)

	err = m.runEncoder(job)
	if err != nil {
		log.Printf("Job %d failed: %v", job.ID, err)
		db.UpdateJobStatus(job.ID, "failed", err.Error())
		db.AddJobReport(job, "failed", 0, 0, 0)
		db.DeleteJob(job.ID)
		m.NotifySSE("job_failed", map[string]interface{}{"id": job.ID, "error": err.Error()})
	} else {
		log.Printf("Job %d succeeded", job.ID)
		db.DeleteJob(job.ID)
		m.NotifySSE("job_completed", map[string]interface{}{"id": job.ID})
	}
}

func (m *Manager) runEncoder(job db.Job) error {
	startTime := time.Now()

	if _, err := os.Stat(job.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("source file missing")
	}

	if err := os.MkdirAll(m.TempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	originalSize := job.OriginalSize
	if originalSize == 0 {
		info, err := os.Stat(job.FilePath)
		if err == nil {
			originalSize = info.Size()
		}
	}

	ext := filepath.Ext(job.FilePath)
	var outExt string
	if job.MediaType == "video" {
		outExt = ".mkv"
	} else {
		outExt = ".mp3"
	}
	
	baseName := strings.TrimSuffix(filepath.Base(job.FilePath), ext)
	tempOutPath := filepath.Join(m.TempDir, fmt.Sprintf("temp_%d_%s%s", job.ID, baseName, outExt))
	finalOutPath := filepath.Join(filepath.Dir(job.FilePath), baseName+outExt)

	duration, _ := m.encoder.ProbeDuration(job.FilePath)

	tempInPath := filepath.Join(m.TempDir, fmt.Sprintf("temp_in_%d_%s", job.ID, filepath.Base(job.FilePath)))

	var cmdErr error
	var execCmd *exec.Cmd

	if job.MediaType == "video" {
		if m.encoder.ShouldSkipVideo(job.FilePath, job.TargetResolution) {
			log.Printf("Skipping video %d - already target codec/resolution", job.ID)
			return db.AddJobReport(job, "skipped", originalSize, 0, 0)
		}
		
		w, h, err := m.encoder.ProbeResolution(job.FilePath)
		if err != nil {
			return fmt.Errorf("failed to probe resolution: %w", err)
		}

		log.Printf("Copying %s to %s before encoding...", job.FilePath, tempInPath)
		if err := copyFile(job.FilePath, tempInPath); err != nil {
			return fmt.Errorf("failed to copy source to temp: %w", err)
		}
		defer os.Remove(tempInPath)

		execCmd, err = m.encoder.BuildVideoCmd(tempInPath, tempOutPath, job.TargetResolution, job.FFmpegFlags, w, h)
		if err != nil {
			return err
		}
	} else {
		if m.encoder.ShouldSkipAudio(job.FilePath) {
			log.Printf("Skipping audio %d - already target codec/bitrate", job.ID)
			return db.AddJobReport(job, "skipped", originalSize, 0, 0)
		}
		
		log.Printf("Copying %s to %s before encoding...", job.FilePath, tempInPath)
		if err := copyFile(job.FilePath, tempInPath); err != nil {
			return fmt.Errorf("failed to copy source to temp: %w", err)
		}
		defer os.Remove(tempInPath)

		execCmd, cmdErr = m.encoder.BuildAudioCmd(tempInPath, tempOutPath, job.FFmpegFlags)
		if cmdErr != nil {
			return cmdErr
		}
	}

	stderr, err := execCmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := execCmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	scanner := bufio.NewScanner(stderr)
	scanner.Split(bufio.ScanLines)
	// Some ffmpeg progress output uses carriage returns instead of newlines
	// We handle standard lines, to get a more robust carriage return split, we could implement a custom split function.
	// For simplicity, ScanLines usually catches 'time=' lines well enough if they have \n.
	// Let's use a custom split to handle '\r' as well.
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.IndexByte(data, '\r'); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	var lastErrLine string
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			lastErrLine = line
			if duration > 0 {
				prog := encoder.ParseProgress(line, duration)
				if prog >= 0 {
					m.NotifySSE("progress", map[string]interface{}{
						"id": job.ID,
						"progress": fmt.Sprintf("%.1f", prog),
					})
				}
			}
		}
	}()

	if err := execCmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg error: %v, last output: %s", err, lastErrLine)
	}

	// Validate format integrity
	if err := m.encoder.ValidateFile(tempOutPath); err != nil {
		os.Remove(tempOutPath)
		return fmt.Errorf("validation failed: %w", err)
	}

	// Validate duration (ensure it wasn't cut short)
	if duration > 0 {
		outDuration, err := m.encoder.ProbeDuration(tempOutPath)
		if err != nil {
			os.Remove(tempOutPath)
			return fmt.Errorf("failed to probe output duration: %w", err)
		}
		
		// Allow up to 5 seconds of difference (sometimes containers/padding vary slightly)
		diff := duration - outDuration
		if diff < 0 {
			diff = -diff
		}
		
		if diff > 5.0 {
			os.Remove(tempOutPath)
			return fmt.Errorf("duration mismatch: original is %.2fs, encoded is %.2fs", duration, outDuration)
		}
	}

	// Calculate sizes
	outInfo, err := os.Stat(tempOutPath)
	if err != nil {
		return fmt.Errorf("failed to stat output: %w", err)
	}
	encodedSize := outInfo.Size()
	sizeSaved := originalSize - encodedSize

	// Move file
	log.Printf("Copying encoded file back to %s...", finalOutPath)
	if err := os.Rename(tempOutPath, finalOutPath); err != nil {
		// Fallback to copy if cross-device link error
		if err := copyFile(tempOutPath, finalOutPath); err != nil {
			return fmt.Errorf("failed to move output: %w", err)
		}
		os.Remove(tempOutPath)
	}

	// Delete original if different extension
	if finalOutPath != job.FilePath {
		os.Remove(job.FilePath)
	}

	processTime := time.Since(startTime).Seconds()
	return db.AddJobReport(job, "success", encodedSize, sizeSaved, processTime)
}
