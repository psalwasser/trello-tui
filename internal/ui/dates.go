package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// parseDue parses a human due date. Supported: today, tomorrow, mon..sun,
// +3d / +2w, 2026-10-15, 10-15, 10/15, optionally followed by a time such
// as 14:30, 2pm or 9am. Dates without a time default to 09:00 local.
func parseDue(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return time.Time{}, fmt.Errorf("empty date")
	}
	hour, min := 9, 0
	if len(fields) > 1 {
		h, m, err := parseClock(fields[len(fields)-1])
		if err != nil {
			return time.Time{}, err
		}
		hour, min = h, m
		fields = fields[:len(fields)-1]
	} else if h, m, err := parseClock(fields[0]); err == nil && strings.ContainsAny(fields[0], ":apm") && !strings.Contains(fields[0], "-") {
		// Bare time means today at that time.
		return time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location()), nil
	}
	d := strings.Join(fields, " ")
	today := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
	switch {
	case d == "today" || d == "tod":
		return today, nil
	case d == "tomorrow" || d == "tom" || d == "tmr":
		return today.AddDate(0, 0, 1), nil
	case strings.HasPrefix(d, "+") && len(d) > 2:
		n, err := strconv.Atoi(d[1 : len(d)-1])
		if err != nil {
			return time.Time{}, fmt.Errorf("bad offset %q", d)
		}
		switch d[len(d)-1] {
		case 'd':
			return today.AddDate(0, 0, n), nil
		case 'w':
			return today.AddDate(0, 0, 7*n), nil
		case 'm':
			return today.AddDate(0, n, 0), nil
		}
		return time.Time{}, fmt.Errorf("bad offset %q", d)
	}
	if len(d) >= 3 {
		if wd, ok := weekdays[d[:3]]; ok {
			delta := (int(wd) - int(now.Weekday()) + 7) % 7
			if delta == 0 {
				delta = 7
			}
			return today.AddDate(0, 0, delta), nil
		}
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02", "01-02", "01/02", "1/2", "1-2", "Jan 2", "2 Jan"} {
		t, err := time.ParseInLocation(layout, d, now.Location())
		if err != nil {
			continue
		}
		year := t.Year()
		if year == 0 {
			year = now.Year()
			if time.Date(year, t.Month(), t.Day(), 23, 59, 0, 0, now.Location()).Before(now) {
				year++
			}
		}
		return time.Date(year, t.Month(), t.Day(), hour, min, 0, 0, now.Location()), nil
	}
	return time.Time{}, fmt.Errorf("can't parse date %q", s)
}

func parseClock(s string) (int, int, error) {
	pm := strings.HasSuffix(s, "pm")
	am := strings.HasSuffix(s, "am")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "pm"), "am")
	parts := strings.SplitN(s, ":", 2)
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("bad time")
	}
	m := 0
	if len(parts) == 2 {
		if m, err = strconv.Atoi(parts[1]); err != nil {
			return 0, 0, fmt.Errorf("bad time")
		}
	}
	if pm && h < 12 {
		h += 12
	}
	if am && h == 12 {
		h = 0
	}
	if h > 23 || m > 59 || (!am && !pm && len(parts) == 1) {
		return 0, 0, fmt.Errorf("bad time")
	}
	return h, m, nil
}

// fmtDue renders a compact due date like "Oct 12" or "Tue 14:00".
func fmtDue(t time.Time, now time.Time) string {
	t = t.Local()
	days := int(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local).Sub(
		time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)).Hours() / 24)
	var d string
	switch {
	case days == 0:
		d = "Today"
	case days == 1:
		d = "Tomorrow"
	case days == -1:
		d = "Yesterday"
	case days > 1 && days < 7:
		d = t.Format("Mon")
	case t.Year() == now.Year():
		d = t.Format("Jan 2")
	default:
		d = t.Format("Jan 2 2006")
	}
	if t.Hour() != 0 || t.Minute() != 0 {
		if t.Minute() == 0 {
			d += " " + strings.ToLower(t.Format("3pm"))
		} else {
			d += " " + t.Format("15:04")
		}
	}
	return d
}

// dueStyle colors a due badge like the web app: green when complete, red
// when overdue, yellow when due within a day.
func dueStyle(t time.Time, complete bool, now time.Time) lipgloss.Style {
	switch {
	case complete:
		return lipgloss.NewStyle().Foreground(cGreen)
	case t.Before(now):
		return lipgloss.NewStyle().Foreground(cRed).Bold(true)
	case t.Sub(now) < 24*time.Hour:
		return lipgloss.NewStyle().Foreground(cYellow)
	default:
		return lipgloss.NewStyle().Foreground(cFg)
	}
}
