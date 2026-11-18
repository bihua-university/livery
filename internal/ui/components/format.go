package components

import (
	"fmt"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

// FormatRate formats bytes per second into a compact tabular string.
func FormatRate(bps int64) string {
	return FormatBytes(bps) + "/s"
}

// FormatBytes formats byte counts cleanly.
func FormatBytes(b int64) string {
	if b < 0 {
		b = 0
	}
	const unit = 1024.0
	f := float64(b)
	switch {
	case f < unit:
		return fmt.Sprintf("%d B", b)
	case f < unit*unit:
		return fmt.Sprintf("%.1f KB", f/unit)
	case f < unit*unit*unit:
		return fmt.Sprintf("%.2f MB", f/(unit*unit))
	default:
		return fmt.Sprintf("%.2f GB", f/(unit*unit*unit))
	}
}

// FormatLatency formats node latency in milliseconds.
func FormatLatency(delay int, testing bool, timeoutLabel string) string {
	if testing {
		return "..."
	}
	if delay < 0 {
		return "--"
	}
	if delay == 0 {
		return timeoutLabel
	}
	return fmt.Sprintf("%d ms", delay)
}

// FormatRelativeTime formats a timestamp relative to now in the active language.
func FormatRelativeTime(t time.Time, lang model.LanguageMode) string {
	if t.IsZero() {
		if lang == model.LangEN {
			return "Never"
		}
		return "从未"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		if lang == model.LangEN {
			return "Just now"
		}
		return "刚刚"
	case d < time.Hour:
		m := int(d.Minutes())
		if lang == model.LangEN {
			return fmt.Sprintf("%dm ago", m)
		}
		return fmt.Sprintf("%d 分钟前", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if lang == model.LangEN {
			return fmt.Sprintf("%dh ago", h)
		}
		return fmt.Sprintf("%d 小时前", h)
	default:
		return t.Format("01-02 15:04")
	}
}

// FormatUptime formats elapsed duration into a concise tabular clock string.
func FormatUptime(startedAt time.Time) string {
	if startedAt.IsZero() {
		return "--"
	}
	d := max(time.Since(startedAt).Round(time.Second), 0)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	sec := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}
