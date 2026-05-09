package marketstate

import (
	"sort"
	"strings"
	"time"
)

type SessionContext struct {
	PrimarySession string
	Phase          string
	Tags           []string
	IsUSOpen       bool
}

var (
	tokyoLocation   = mustLoadLocation("Asia/Tokyo")
	londonLocation  = mustLoadLocation("Europe/London")
	newYorkLocation = mustLoadLocation("America/New_York")
)

func BuildSessionContext(now time.Time) SessionContext {
	now = now.UTC()
	if isWeekendOffHours(now) {
		return SessionContext{
			PrimarySession: "OFF_HOURS",
			Phase:          "weekend",
			Tags:           []string{"OFF_HOURS", "WEEKEND"},
			IsUSOpen:       false,
		}
	}

	tokyo := now.In(tokyoLocation)
	london := now.In(londonLocation)
	newYork := now.In(newYorkLocation)

	inAsia := isTimeWindow(tokyo, 9, 0, 15, 0)
	inLondon := isTimeWindow(london, 8, 0, 16, 30)
	inUS := isTimeWindow(newYork, 9, 30, 16, 0)
	isUSOpen := isTimeWindow(newYork, 9, 30, 10, 30)

	tags := make([]string, 0, 6)
	if inAsia {
		tags = append(tags, "ASIA", "TOKYO")
	}
	if inLondon {
		tags = append(tags, "LONDON")
	}
	if inUS {
		tags = append(tags, "US")
	}
	if isUSOpen {
		tags = append(tags, "US_OPEN")
	}
	if inLondon && inUS {
		tags = append(tags, "LONDON_US_OVERLAP")
	}
	if inAsia && inLondon {
		tags = append(tags, "ASIA_LONDON_HANDOFF")
	}

	ctx := SessionContext{
		PrimarySession: primarySession(inAsia, inLondon, inUS, isUSOpen),
		Phase:          sessionPhase(inAsia, inLondon, inUS, isUSOpen),
		Tags:           dedupeAndSortTags(tags),
		IsUSOpen:       isUSOpen,
	}
	if ctx.PrimarySession == "" {
		ctx.PrimarySession = "OFF_HOURS"
	}
	if ctx.Phase == "" {
		ctx.Phase = "mid"
	}
	return ctx
}

func sessionPhase(inAsia, inLondon, inUS, isUSOpen bool) string {
	switch {
	case isUSOpen:
		return "open"
	case inLondon && inUS:
		return "overlap"
	case inAsia && inLondon:
		return "handoff"
	case inAsia || inLondon || inUS:
		return "mid"
	default:
		return "off_hours"
	}
}

func primarySession(inAsia, inLondon, inUS, isUSOpen bool) string {
	switch {
	case isUSOpen:
		return "US_OPEN"
	case inLondon && inUS:
		return "LONDON_US_OVERLAP"
	case inAsia && inLondon:
		return "ASIA_LONDON_HANDOFF"
	case inUS:
		return "US"
	case inLondon:
		return "LONDON"
	case inAsia:
		return "ASIA"
	default:
		return "OFF_HOURS"
	}
}

func dedupeAndSortTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToUpper(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func isTimeWindow(t time.Time, startHour, startMinute, endHour, endMinute int) bool {
	mins := t.Hour()*60 + t.Minute()
	start := startHour*60 + startMinute
	end := endHour*60 + endMinute
	return mins >= start && mins < end
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

func isWeekendOffHours(now time.Time) bool {
	now = now.UTC()
	if now.Weekday() == time.Saturday {
		return true
	}
	if now.Weekday() != time.Friday {
		return false
	}
	ny := now.In(newYorkLocation)
	fridayCloseNY := time.Date(ny.Year(), ny.Month(), ny.Day(), 16, 0, 0, 0, newYorkLocation)
	return !ny.Before(fridayCloseNY)
}
