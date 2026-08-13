package queue

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"goencode/internal/db"
	"goencode/internal/encoder"
	"goencode/internal/notify"
)

func copyFile(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

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

	buf := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			os.Remove(dst)
			return err
		}
		n, readErr := in.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
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
	if m.isShuttingDown() {
		return
	}

	var jobID int
	var hasJobs bool
	defer func() {
		if jobID != 0 {
			m.endJob(jobID)
		}
		if hasJobs && !m.isShuttingDown() {
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
	foundPending := false
	for _, j := range jobs {
		if j.Status == "pending" {
			job = j
			foundPending = true
			break
		}
	}
	if !foundPending {
		hasJobs = false
		return
	}

	claimed, err := db.ClaimJob(job.ID)
	if err != nil {
		log.Printf("Failed to claim job %d: %v", job.ID, err)
		return
	}
	if !claimed {
		return
	}

	jobID = job.ID
	m.Trigger()

	ctx := m.beginJob(job.ID)
	log.Printf("Starting job %d for %s", job.ID, job.FilePath)

	job.Status = "processing"
	job.UpdatedAt = time.Now()
	m.NotifySSE("job_started", job)

	err = m.runEncoder(ctx, job)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		m.cleanupTemps(job.ID)
		if m.isShuttingDown() {
			log.Printf("Job %d interrupted by shutdown", job.ID)
			job.ErrorMessage = "Interrupted by shutdown"
			_ = db.AddJobReport(job, "failed", 0, 0, 0)
			_ = db.DeleteJob(job.ID)
			return
		}
		log.Printf("Job %d cancelled", job.ID)
		_ = db.DeleteJob(job.ID)
		m.NotifySSE("job_cancelled", map[string]interface{}{"id": job.ID})
		m.NotifySSE("queue_updated", nil)
		return
	}
	if err != nil {
		log.Printf("Job %d failed: %v", job.ID, err)
		db.UpdateJobStatus(job.ID, "failed", err.Error())
		db.AddJobReport(job, "failed", 0, 0, 0)
		db.DeleteJob(job.ID)
		m.NotifySSE("job_failed", map[string]interface{}{"id": job.ID, "error": err.Error()})
		if m.WebhookURL != "" {
			notify.SendWebhook(m.WebhookURL, "Encoding Job Failed", fmt.Sprintf("File: %s\nError: %s", job.FilePath, err.Error()))
		}
	} else {
		log.Printf("Job %d succeeded", job.ID)
		db.DeleteJob(job.ID)
		m.NotifySSE("job_completed", map[string]interface{}{"id": job.ID})
	}
}

func (m *Manager) runEncoder(ctx context.Context, job db.Job) error {
	startTime := time.Now()
	defer m.cleanupTemps(job.ID)

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
	outExt := job.OutputExt(job.MediaType)
	baseName := strings.TrimSuffix(filepath.Base(job.FilePath), ext)
	tempOutPath := filepath.Join(m.TempDir, fmt.Sprintf("temp_%d_%s%s", job.ID, baseName, outExt))

	outDir := filepath.Dir(job.FilePath)
	if strings.TrimSpace(job.OutputDir) != "" {
		outDir = strings.TrimSpace(job.OutputDir)
	}
	finalOutPath := filepath.Join(outDir, baseName+outExt)

	duration, _ := m.encoder.ProbeDuration(job.FilePath)

	tempInPath := filepath.Join(m.TempDir, fmt.Sprintf("temp_in_%d_%s", job.ID, filepath.Base(job.FilePath)))
	m.addTemps(job.ID, tempInPath, tempOutPath)

	if err := ctx.Err(); err != nil {
		return err
	}

	var cmdErr error
	var execCmd *exec.Cmd

	if job.Force {
		log.Printf("Force encoding job %d, skip checks disabled", job.ID)
	}

	if job.MediaType == "video" {
		if !job.Force {
			if skip, reason := encoder.CheckVideoSkip(job.FilePath, job.TargetResolution, job.VideoCodec); skip {
				log.Printf("Skipping video %d - %s", job.ID, reason)
				job.ErrorMessage = reason
				return db.AddJobReport(job, "skipped", originalSize, 0, 0)
			}
		}

		w, h, err := m.encoder.ProbeResolution(job.FilePath)
		if err != nil {
			return fmt.Errorf("failed to probe resolution: %w", err)
		}

		log.Printf("Copying %s to %s before encoding...", job.FilePath, tempInPath)
		if err := copyFile(ctx, job.FilePath, tempInPath); err != nil {
			return fmt.Errorf("failed to copy source to temp: %w", err)
		}

		execCmd, err = m.encoder.BuildVideoCmd(tempInPath, tempOutPath, encoder.VideoEncodeOptions{
			TargetResolution: job.TargetResolution,
			VideoCodec:       job.VideoCodec,
			AudioCodec:       job.AudioCodec,
			CRF:              job.CRF,
			Preset:           job.Preset,
			Tune:             job.Tune,
			Profile:          job.Profile,
			Container:        job.Container,
			CustomFlags:      job.FFmpegFlags,
			OriginalWidth:    w,
			OriginalHeight:   h,
			KeepExtraStreams: job.KeepExtraStreams,
		})
		if err != nil {
			return err
		}
	} else {
		if !job.Force {
			if skip, reason := encoder.CheckAudioSkip(job.FilePath); skip {
				log.Printf("Skipping audio %d - %s", job.ID, reason)
				job.ErrorMessage = reason
				return db.AddJobReport(job, "skipped", originalSize, 0, 0)
			}
		}

		log.Printf("Copying %s to %s before encoding...", job.FilePath, tempInPath)
		if err := copyFile(ctx, job.FilePath, tempInPath); err != nil {
			return fmt.Errorf("failed to copy source to temp: %w", err)
		}

		execCmd, cmdErr = m.encoder.BuildAudioCmd(tempInPath, tempOutPath, job.FFmpegFlags)
		if cmdErr != nil {
			return cmdErr
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	prepareCmd(execCmd)

	stderr, err := execCmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := execCmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	m.setCurrentCmd(job.ID, execCmd)
	if ctx.Err() != nil {
		interruptCmd(execCmd)
		go escalateKill(execCmd, m.cmdStillCurrent)
		_ = execCmd.Wait()
		return ctx.Err()
	}

	scanner := bufio.NewScanner(stderr)
	scanner.Split(bufio.ScanLines)
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
						"id":       job.ID,
						"progress": fmt.Sprintf("%.1f", prog),
					})
				}
			}
		}
	}()

	if err := execCmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg error: %v, last output: %s", err, lastErrLine)
	}
	if err := ctx.Err(); err != nil {
		return err
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

	if job.KeepOriginalIfLarger && encodedSize >= originalSize {
		os.Remove(tempOutPath)
		job.ErrorMessage = fmt.Sprintf("Encoded file larger than original (%s vs %s)", formatBytes(encodedSize), formatBytes(originalSize))
		log.Printf("Keeping original for job %d: %s", job.ID, job.ErrorMessage)
		return db.AddJobReport(job, "skipped", encodedSize, 0, time.Since(startTime).Seconds())
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	log.Printf("Copying encoded file back to %s...", finalOutPath)
	if err := os.Rename(tempOutPath, finalOutPath); err != nil {
		if err := copyFile(ctx, tempOutPath, finalOutPath); err != nil {
			return fmt.Errorf("failed to move output: %w", err)
		}
		os.Remove(tempOutPath)
	}

	outputElsewhere := strings.TrimSpace(job.OutputDir) != ""
	if outputElsewhere {
		if job.DeleteSource {
			os.Remove(job.FilePath)
		}
	} else if finalOutPath != job.FilePath {
		os.Remove(job.FilePath)
	}

	processTime := time.Since(startTime).Seconds()
	return db.AddJobReport(job, "success", encodedSize, sizeSaved, processTime)
}

func formatBytes(n int64) string {
	const mb = 1024 * 1024
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
	const kb = 1024
	if n >= kb {
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	}
	return fmt.Sprintf("%d B", n)
}
