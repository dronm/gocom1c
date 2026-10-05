package logging

import (
	"fmt"
	"math/big"
	"strings"
	"time"
)

const DefaultRotationPeriod = "daily"

// Period describes either a local calendar day or a fixed interval anchored at
// the Unix epoch. Construct it with ParsePeriod.
type Period struct {
	daily       bool
	interval    time.Duration
	epochOffset time.Duration
}

// ParsePeriod accepts "daily" or any positive duration accepted by
// time.ParseDuration. An empty setting selects the daily default.
func ParsePeriod(value string) (Period, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, DefaultRotationPeriod) {
		return Period{daily: true}, nil
	}

	interval, err := time.ParseDuration(value)
	if err != nil || interval <= 0 {
		return Period{}, fmt.Errorf("invalid log rotation period %q: use daily or a positive duration such as 1h or 24h", value)
	}

	// Time.Truncate is anchored at Go's zero time, rather than the Unix epoch.
	// Calculate the anchor adjustment once, without overflowing a Duration or
	// UnixNano, so fixed periods also work outside UnixNano's 1678-2262 range.
	epochNanoseconds := new(big.Int).Mul(big.NewInt(62135596800), big.NewInt(int64(time.Second)))
	offset := new(big.Int).Mod(epochNanoseconds, big.NewInt(int64(interval)))
	return Period{
		interval:    interval,
		epochOffset: time.Duration(offset.Int64()),
	}, nil
}

func (p Period) window(now time.Time) (time.Time, time.Time) {
	if p.daily {
		start := calendarDayStart(now.Year(), now.Month(), now.Day(), now.Location())
		end := calendarDayStart(now.Year(), now.Month(), now.Day()+1, now.Location())
		return start, end
	}

	start := now.UTC().Add(-p.epochOffset).Truncate(p.interval).Add(p.epochOffset)
	return start, start.Add(p.interval)
}

func calendarDayStart(year int, month time.Month, day int, location *time.Location) time.Time {
	// Normalize the requested date without a timezone transition first. In some
	// locations midnight is skipped; Time.Date may then select the previous date.
	year, month, day = time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Date()
	start := time.Date(year, month, day, 0, 0, 0, 0, location)
	startYear, startMonth, startDay := start.Date()
	if startYear != year || startMonth != month || startDay != day {
		_, transition := start.ZoneBounds()
		if !transition.IsZero() {
			return transition
		}
	}
	return start
}

func (p Period) suffix(start time.Time) string {
	if p.daily {
		return start.Format("2006-01-02")
	}

	return fmt.Sprintf("%s-every-%dns", start.UTC().Format("2006-01-02T15-04-05.000000000Z"), p.interval)
}
