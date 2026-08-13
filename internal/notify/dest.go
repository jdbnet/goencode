package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

type webhookDest struct {
	url string
}

type webhookPayload struct {
	Title          string  `json:"title"`
	Message        string  `json:"message"`
	Time           string  `json:"time"`
	Event          string  `json:"event"`
	FilePath       string  `json:"file_path"`
	MediaType      string  `json:"media_type"`
	OriginalSize   int64   `json:"original_size"`
	EncodedSize    int64   `json:"encoded_size"`
	SizeSaved      int64   `json:"size_saved"`
	ProcessingTime float64 `json:"processing_time"`
	Detail         string  `json:"detail,omitempty"`
}

func (d webhookDest) send(e Event) error {
	payload := webhookPayload{
		Title:          e.Title(),
		Message:        e.Message(),
		Time:           rfc3339Now(),
		Event:          e.Kind,
		FilePath:       e.FilePath,
		MediaType:      e.MediaType,
		OriginalSize:   e.OriginalSize,
		EncodedSize:    e.EncodedSize,
		SizeSaved:      e.SizeSaved,
		ProcessingTime: e.ProcessingTime,
		Detail:         e.Detail,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return post(d.url, "application/json", nil, body)
}

type discordDest struct {
	url string
}

func (d discordDest) send(e Event) error {
	color := 0x21d198
	switch e.Kind {
	case KindSkip:
		color = 0xf59e0b
	case KindFailed:
		color = 0xef4444
	}
	embed := map[string]interface{}{
		"title":       e.Title(),
		"description": e.Message(),
		"color":       color,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(map[string]interface{}{
		"username": "GoEncode",
		"embeds":   []map[string]interface{}{embed},
	})
	if err != nil {
		return err
	}
	return post(d.url, "application/json", nil, body)
}

type ntfyDest struct {
	url   string
	token string
}

func (d ntfyDest) send(e Event) error {
	headers := map[string]string{
		"Title":    e.Title(),
		"Tags":     ntfyTags(e.Kind),
		"Priority": ntfyPriority(e.Kind),
	}
	if d.token != "" {
		headers["Authorization"] = "Bearer " + d.token
	}
	return post(d.url, "text/plain; charset=utf-8", headers, []byte(e.Message()))
}

func ntfyTags(kind string) string {
	switch kind {
	case KindSuccess:
		return "white_check_mark"
	case KindSkip:
		return "fast_forward"
	default:
		return "x"
	}
}

func ntfyPriority(kind string) string {
	switch kind {
	case KindFailed:
		return "4"
	case KindSkip:
		return "2"
	default:
		return "3"
	}
}

type gotifyDest struct {
	url   string
	token string
}

func (d gotifyDest) send(e Event) error {
	endpoint := strings.TrimRight(d.url, "/") + "/message"
	if d.token != "" {
		endpoint += "?token=" + url.QueryEscape(d.token)
	}
	payload := map[string]interface{}{
		"title":    e.Title(),
		"message":  e.Message(),
		"priority": gotifyPriority(e.Kind),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	headers := map[string]string{}
	if d.token != "" {
		headers["X-Gotify-Key"] = d.token
	}
	return post(endpoint, "application/json", headers, body)
}

func gotifyPriority(kind string) int {
	switch kind {
	case KindFailed:
		return 8
	case KindSkip:
		return 2
	default:
		return 5
	}
}

func post(rawURL, contentType string, headers map[string]string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s returned %d", rawURL, resp.StatusCode)
	}
	return nil
}
