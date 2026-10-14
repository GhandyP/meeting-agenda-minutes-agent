// Package calendar renders deterministic, local-only calendar artifacts.
package calendar

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/approval"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
)

// ICS renders an agenda as deterministic RFC 5545-style calendar text. It
// intentionally contains no clock-derived date or time values.
func ICS(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) ([]byte, error) {
	if in.Mode != meeting.AgendaMode {
		return nil, fmt.Errorf("calendar artifact requires agenda input mode, got %q", in.Mode)
	}
	if out.Mode != in.Mode {
		return nil, fmt.Errorf("calendar input/output mode mismatch: %q and %q", in.Mode, out.Mode)
	}
	if in.DurationMinutes <= 0 {
		return nil, errors.New("calendar request duration must be positive")
	}
	if len(out.Topics) == 0 {
		return nil, errors.New("calendar agenda must contain at least one topic")
	}

	title := out.Title
	if title == "" {
		title = in.Title
	}
	payloadHash, err := approval.Hash(in, out)
	if err != nil {
		return nil, fmt.Errorf("hash calendar proposal: %w", err)
	}
	uid := payloadHash + "@meeting-agenda-minutes-agent"
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//GhandyP//Meeting Agenda Minutes Agent//EN",
		"X-DRY-RUN:TRUE",
		"BEGIN:VEVENT",
		"UID:" + uid,
		"SUMMARY:" + escapeText(title),
		fmt.Sprintf("DURATION:PT%dM", in.DurationMinutes),
	}
	for _, topic := range out.Topics {
		lines = append(lines, fmt.Sprintf("X-TOPIC:%s (%d minutes)", escapeText(topic.Title), topic.Minutes))
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")

	var text strings.Builder
	for _, line := range lines {
		text.WriteString(foldLine(line))
		text.WriteString("\r\n")
	}
	return []byte(text.String()), nil
}

// WriteDryRun writes one approved, deterministic ICS artifact under dir.
func WriteDryRun(dir string, g approval.Gate, token string, in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (string, error) {
	if err := g.Authorize(token, in, out); err != nil {
		return "", err
	}
	name, err := approval.ArtifactFileName(in.MeetingID, "ics")
	if err != nil {
		return "", err
	}
	data, err := ICS(in, out)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create calendar artifact directory: %w", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write calendar artifact: %w", err)
	}
	return path, nil
}

func escapeText(value string) string {
	return strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\r", "\\n",
		"\n", "\\n",
	).Replace(value)
}

func foldLine(line string) string {
	remaining := []byte(line)
	var folded strings.Builder
	limit := 75
	for len(remaining) > limit {
		cut := limit
		for cut > 0 && cut < len(remaining) && !utf8.RuneStart(remaining[cut]) {
			cut--
		}
		if cut == 0 {
			cut = limit
		}
		folded.Write(remaining[:cut])
		folded.WriteString("\r\n ")
		remaining = remaining[cut:]
		limit = 74
	}
	folded.Write(remaining)
	return folded.String()
}
