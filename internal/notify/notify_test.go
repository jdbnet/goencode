package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEventTitleAndMessage(t *testing.T) {
	e := Event{
		Kind:           KindSuccess,
		FilePath:       "/media/movies/Film.mkv",
		OriginalSize:   82 << 30,
		EncodedSize:    42 << 30,
		SizeSaved:      40 << 30,
		ProcessingTime: 4350,
	}
	if got := e.Title(); got != "Saved 40.0 GB" {
		t.Fatalf("Title = %q, want Saved 40.0 GB", got)
	}
	msg := e.Message()
	if !strings.Contains(msg, "Film.mkv") || !strings.Contains(msg, "Saved 40.0 GB") {
		t.Fatalf("Message = %q", msg)
	}
	if !strings.Contains(msg, "1h 12m") {
		t.Fatalf("expected duration in message: %q", msg)
	}

	skip := Event{Kind: KindSkip, FilePath: "/a/b.mkv", Detail: "Already HEVC"}
	if skip.Title() != "Encode skipped" {
		t.Fatalf("skip title = %q", skip.Title())
	}
	if !strings.Contains(skip.Message(), "Already HEVC") {
		t.Fatalf("skip message = %q", skip.Message())
	}
}

func TestParseEventsDefaultAndFilter(t *testing.T) {
	def := parseEvents(nil)
	if !def[KindSuccess] || !def[KindSkip] || !def[KindFailed] {
		t.Fatalf("default events = %#v", def)
	}
	only := parseEvents([]string{"failed", " SUCCESS "})
	if !only[KindFailed] || !only[KindSuccess] || only[KindSkip] {
		t.Fatalf("filtered events = %#v", only)
	}
	csv := parseEvents([]string{"success,skip"})
	if !csv[KindSuccess] || !csv[KindSkip] || csv[KindFailed] {
		t.Fatalf("csv events = %#v", csv)
	}
}

func TestWebhookAndNtfyAndGotify(t *testing.T) {
	var webhookBody, ntfyBody, gotifyBody []byte
	var ntfyTitle, ntfyTags, gotifyPath string

	mux := http.NewServeMux()
	mux.HandleFunc("/hook", func(w http.ResponseWriter, r *http.Request) {
		webhookBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/ntfy", func(w http.ResponseWriter, r *http.Request) {
		ntfyTitle = r.Header.Get("Title")
		ntfyTags = r.Header.Get("Tags")
		ntfyBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		gotifyPath = r.URL.RequestURI()
		gotifyBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := Event{
		Kind:         KindSuccess,
		FilePath:     "/media/Film.mkv",
		MediaType:    "video",
		OriginalSize: 100,
		EncodedSize:  40,
		SizeSaved:    60,
	}

	if err := (webhookDest{url: srv.URL + "/hook"}).send(e); err != nil {
		t.Fatal(err)
	}
	var payload webhookPayload
	if err := json.Unmarshal(webhookBody, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Event != KindSuccess || payload.SizeSaved != 60 || payload.Title == "" {
		t.Fatalf("webhook payload = %+v", payload)
	}

	if err := (ntfyDest{url: srv.URL + "/ntfy"}).send(e); err != nil {
		t.Fatal(err)
	}
	if ntfyTitle != e.Title() || ntfyTags != "white_check_mark" {
		t.Fatalf("ntfy headers title=%q tags=%q", ntfyTitle, ntfyTags)
	}
	if !strings.Contains(string(ntfyBody), "Film.mkv") {
		t.Fatalf("ntfy body = %s", ntfyBody)
	}

	if err := (gotifyDest{url: srv.URL, token: "app-token"}).send(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotifyPath, "token=app-token") {
		t.Fatalf("gotify path = %s", gotifyPath)
	}
	if !strings.Contains(string(gotifyBody), `"priority":5`) {
		t.Fatalf("gotify body = %s", gotifyBody)
	}
}

func TestDiscordPayload(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	e := Event{Kind: KindFailed, FilePath: "/a.mkv", Detail: "ffmpeg error"}
	if err := (discordDest{url: srv.URL}).send(e); err != nil {
		t.Fatal(err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["username"] != "GoEncode" {
		t.Fatalf("username = %v", body["username"])
	}
	embeds := body["embeds"].([]interface{})
	embed := embeds[0].(map[string]interface{})
	if embed["title"] != "Encode failed" {
		t.Fatalf("embed title = %v", embed["title"])
	}
	if int(embed["color"].(float64)) != 0xef4444 {
		t.Fatalf("embed color = %v", embed["color"])
	}
}

func TestNewAutoDetectsDiscordWebhook(t *testing.T) {
	n := New(Options{WebhookURL: "https://discord.com/api/webhooks/1/abc"})
	if len(n.dests) != 1 {
		t.Fatalf("dests = %d", len(n.dests))
	}
	if _, ok := n.dests[0].(discordDest); !ok {
		t.Fatalf("expected discord dest, got %T", n.dests[0])
	}
}

func TestNotifyRespectsEvents(t *testing.T) {
	n := New(Options{Events: []string{"failed"}, WebhookURL: "http://127.0.0.1:1/unused"})
	if !n.events[KindFailed] || n.events[KindSuccess] {
		t.Fatalf("events = %#v", n.events)
	}
}

func TestFormatBytes(t *testing.T) {
	if got := FormatBytes(40 << 30); got != "40.0 GB" {
		t.Fatalf("40GiB = %q", got)
	}
	if got := FormatBytes(512); got != "512 B" {
		t.Fatalf("512B = %q", got)
	}
}
