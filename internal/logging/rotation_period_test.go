package logging

import (
	"strings"
	"testing"
	"time"
	_ "time/tzdata"
)

func TestParseRotationPeriod(t *testing.T) {
	for _, value := range []string{"", "daily", " DAILY "} {
		period, err := ParsePeriod(value)
		if err != nil || !period.daily {
			t.Errorf("ParsePeriod(%q) = %+v, %v; want daily", value, period, err)
		}
	}

	for _, value := range []string{"1ns", "250ms", "1h", "6h", "24h", " 7h "} {
		period, err := ParsePeriod(value)
		if err != nil || period.daily || period.interval <= 0 {
			t.Errorf("ParsePeriod(%q) = %+v, %v; want a positive fixed period", value, period, err)
		}
	}

	for _, value := range []string{"0", "0s", "-1h", "tomorrow", "1d", "999999999999999999h"} {
		if _, err := ParsePeriod(value); err == nil || !strings.Contains(err.Error(), value) {
			t.Errorf("ParsePeriod(%q) error = %v; want an error identifying the invalid setting", value, err)
		}
	}
}

func TestDailyRotationUsesCalendarDaysAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	period, err := ParsePeriod("daily")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		now  time.Time
		span time.Duration
	}{
		{"spring", time.Date(2026, 3, 8, 12, 0, 0, 0, location), 23 * time.Hour},
		{"autumn", time.Date(2026, 11, 1, 12, 0, 0, 0, location), 25 * time.Hour},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end := period.window(test.now)
			if start.Hour() != 0 || end.Hour() != 0 || start.Location() != location || end.Location() != location {
				t.Fatalf("window = %v to %v; want local midnights", start, end)
			}
			if span := end.Sub(start); span != test.span {
				t.Fatalf("window span = %v; want %v", span, test.span)
			}
			if period.suffix(start) != test.now.Format("2006-01-02") {
				t.Fatalf("suffix = %q; want local calendar date", period.suffix(start))
			}
		})
	}
}

func TestFixedRotationUsesUTCEpochBuckets(t *testing.T) {
	tests := []struct {
		period string
		now    time.Time
		start  time.Time
	}{
		{"7h", time.Date(1970, 1, 1, 8, 0, 0, 0, time.UTC), time.Date(1970, 1, 1, 7, 0, 0, 0, time.UTC)},
		{"7h", time.Date(1969, 12, 31, 23, 59, 0, 0, time.UTC), time.Date(1969, 12, 31, 17, 0, 0, 0, time.UTC)},
		{"250ms", time.Date(2026, 10, 5, 8, 0, 0, 749999999, time.UTC), time.Date(2026, 10, 5, 8, 0, 0, 500000000, time.UTC)},
		{"1ns", time.Date(2500, 1, 1, 8, 0, 0, 123456789, time.UTC), time.Date(2500, 1, 1, 8, 0, 0, 123456789, time.UTC)},
		{"24h", time.Date(2026, 10, 5, 2, 0, 0, 0, time.FixedZone("UTC+3", 3*60*60)), time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)},
	}
	for _, test := range tests {
		t.Run(test.period+"/"+test.now.String(), func(t *testing.T) {
			period, err := ParsePeriod(test.period)
			if err != nil {
				t.Fatal(err)
			}
			start, end := period.window(test.now)
			if !start.Equal(test.start) || start.Location() != time.UTC {
				t.Fatalf("start = %v; want %v in UTC", start, test.start)
			}
			if !end.Equal(test.start.Add(period.interval)) {
				t.Fatalf("end = %v; want start + %v", end, period.interval)
			}
		})
	}
}

func TestDailyRotationWindowHandlesMissingMidnight(t *testing.T) {
	period, err := ParsePeriod("daily")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		zone  string
		year  int
		month time.Month
		day   int
	}{
		{"Africa/Cairo", 2026, time.April, 24},
		{"America/Sao_Paulo", 2018, time.November, 4},
	} {
		t.Run(test.zone, func(t *testing.T) {
			location, err := time.LoadLocation(test.zone)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(test.year, test.month, test.day, 12, 0, 0, 0, location)
			start, end := period.window(now)
			if start.Format("2006-01-02") != now.Format("2006-01-02") || start.Hour() != 1 {
				t.Fatalf("start = %v; want 01:00 on %s", start, now.Format("2006-01-02"))
			}
			wantEnd := time.Date(test.year, test.month, test.day+1, 0, 0, 0, 0, location)
			if !end.Equal(wantEnd) || end.Sub(start) != 23*time.Hour {
				t.Fatalf("end = %v; want %v, 23 hours after start", end, wantEnd)
			}
		})
	}
}

func TestFixedRotationDoesNotOverflowUnixNanoseconds(t *testing.T) {
	period, err := ParsePeriod("7h")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2500, 4, 3, 8, 9, 10, 123456789, time.UTC)
	seconds := now.Unix()
	expected := time.Unix(seconds-seconds%(7*60*60), 0).UTC()
	start, _ := period.window(now)
	if !start.Equal(expected) {
		t.Fatalf("start = %v; want %v", start, expected)
	}
}
