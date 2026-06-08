package notify

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type WebhookPayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

func SendWebhook(url, title, message string) {
	if url == "" {
		return
	}

	payload := WebhookPayload{
		Title:   title,
		Message: message,
		Time:    time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal webhook payload: %v", err)
		return
	}

	go func() {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
		if err != nil {
			log.Printf("Failed to create webhook request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed to send webhook: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			log.Printf("Webhook returned error status: %d", resp.StatusCode)
		}
	}()
}
