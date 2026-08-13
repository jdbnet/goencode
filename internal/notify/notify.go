package notify

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

const (
	KindSuccess = "success"
	KindSkip    = "skip"
	KindFailed  = "failed"
)

type Event struct {
	Kind           string
	FilePath       string
	MediaType      string
	OriginalSize   int64
	EncodedSize    int64
	SizeSaved      int64
	ProcessingTime float64
	Detail         string
}

func (e Event) Title() string {
	switch e.Kind {
	case KindSuccess:
		if e.SizeSaved > 0 {
			return "Saved " + FormatBytes(e.SizeSaved)
		}
		if e.SizeSaved < 0 {
			return "Grew by " + FormatBytes(-e.SizeSaved)
		}
		return "Encode succeeded"
	case KindSkip:
		return "Encode skipped"
	case KindFailed:
		return "Encode failed"
	default:
		return "GoEncode"
	}
}

func (e Event) Message() string {
	var b strings.Builder
	name := filepath.Base(e.FilePath)
	if name != "" && name != "." {
		b.WriteString(name)
		b.WriteByte('\n')
	}
	if e.FilePath != "" {
		b.WriteString(e.FilePath)
	}
	switch e.Kind {
	case KindSuccess:
		b.WriteByte('\n')
		b.WriteString(sizeLine(e.OriginalSize, e.EncodedSize, e.SizeSaved))
		if e.ProcessingTime > 0 {
			b.WriteString(" in ")
			b.WriteString(FormatDuration(e.ProcessingTime))
		}
	case KindSkip:
		if e.Detail != "" {
			b.WriteByte('\n')
			b.WriteString(e.Detail)
		}
	case KindFailed:
		if e.Detail != "" {
			b.WriteByte('\n')
			b.WriteString(e.Detail)
		}
	}
	return strings.TrimSpace(b.String())
}

func sizeLine(original, encoded, saved int64) string {
	from := FormatBytes(original)
	to := FormatBytes(encoded)
	switch {
	case saved > 0:
		return fmt.Sprintf("Saved %s (%s → %s)", FormatBytes(saved), from, to)
	case saved < 0:
		return fmt.Sprintf("Grew by %s (%s → %s)", FormatBytes(-saved), from, to)
	default:
		return fmt.Sprintf("Size unchanged (%s)", from)
	}
}

type Notifier interface {
	Notify(Event)
}

type Options struct {
	Events         []string
	WebhookURL     string
	NtfyURL        string
	NtfyToken      string
	DiscordWebhook string
	GotifyURL      string
	GotifyToken    string
}

type Fanout struct {
	events map[string]bool
	dests  []destination
}

type destination interface {
	send(Event) error
}

func New(opts Options) *Fanout {
	events := parseEvents(opts.Events)
	var dests []destination
	if u := strings.TrimSpace(opts.WebhookURL); u != "" {
		if isDiscordWebhook(u) {
			dests = append(dests, discordDest{url: u})
		} else {
			dests = append(dests, webhookDest{url: u})
		}
	}
	if u := strings.TrimSpace(opts.DiscordWebhook); u != "" && u != strings.TrimSpace(opts.WebhookURL) {
		dests = append(dests, discordDest{url: u})
	}
	if u := strings.TrimSpace(opts.NtfyURL); u != "" {
		dests = append(dests, ntfyDest{url: u, token: strings.TrimSpace(opts.NtfyToken)})
	}
	if u := strings.TrimSpace(opts.GotifyURL); u != "" {
		dests = append(dests, gotifyDest{url: u, token: strings.TrimSpace(opts.GotifyToken)})
	}
	return &Fanout{events: events, dests: dests}
}

func (f *Fanout) Enabled() bool {
	return f != nil && len(f.dests) > 0
}

func (f *Fanout) Notify(e Event) {
	if f == nil || len(f.dests) == 0 {
		return
	}
	if !f.events[e.Kind] {
		return
	}
	for _, d := range f.dests {
		d := d
		go func() {
			if err := d.send(e); err != nil {
				log.Printf("Notification failed: %v", err)
			}
		}()
	}
}

func parseEvents(list []string) map[string]bool {
	out := make(map[string]bool)
	for _, raw := range list {
		for _, part := range strings.Split(raw, ",") {
			kind := strings.ToLower(strings.TrimSpace(part))
			switch kind {
			case KindSuccess, KindSkip, KindFailed:
				out[kind] = true
			}
		}
	}
	if len(out) == 0 {
		out[KindSuccess] = true
		out[KindSkip] = true
		out[KindFailed] = true
	}
	return out
}

func FormatBytes(n int64) string {
	const unit = 1024
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	if n < unit {
		return fmt.Sprintf("%s%d B", sign, n)
	}
	div, exp := int64(unit), 0
	v := n
	for v >= unit*unit && exp < len("KMGTPE")-1 {
		div *= unit
		exp++
		v /= unit
	}
	return fmt.Sprintf("%s%.1f %cB", sign, float64(n)/float64(div), "KMGTPE"[exp])
}

func FormatDuration(seconds float64) string {
	if seconds <= 0 {
		return "0s"
	}
	total := int64(seconds)
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	if seconds < 1 {
		return fmt.Sprintf("%.2fs", seconds)
	}
	return fmt.Sprintf("%.0fs", seconds)
}

func isDiscordWebhook(u string) bool {
	u = strings.ToLower(u)
	return strings.Contains(u, "discord.com/api/webhooks") || strings.Contains(u, "discordapp.com/api/webhooks")
}

func rfc3339Now() string {
	return time.Now().Format(time.RFC3339)
}
