package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogRotationPeriodConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		want    string
		invalid bool
	}{
		{name: "default", json: `{}`, want: "daily"},
		{name: "daily", json: `{"logRotationPeriod":"daily"}`, want: "daily"},
		{name: "hourly", json: `{"logRotationPeriod":"1h"}`, want: "1h"},
		{name: "six hours", json: `{"logRotationPeriod":"6h"}`, want: "6h"},
		{name: "fixed day", json: `{"logRotationPeriod":"24h"}`, want: "24h"},
		{name: "subsecond", json: `{"logRotationPeriod":"250ms"}`, want: "250ms"},
		{name: "empty", json: `{"logRotationPeriod":""}`, invalid: true},
		{name: "zero", json: `{"logRotationPeriod":"0s"}`, invalid: true},
		{name: "negative", json: `{"logRotationPeriod":"-1h"}`, invalid: true},
		{name: "days suffix", json: `{"logRotationPeriod":"1d"}`, invalid: true},
		{name: "number", json: `{"logRotationPeriod":3600}`, invalid: true},
		{name: "boolean", json: `{"logRotationPeriod":true}`, invalid: true},
		{name: "null", json: `{"logRotationPeriod":null}`, invalid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(filename, []byte(tc.json), 0o600); err != nil {
				t.Fatal(err)
			}
			var cfg Config
			err := cfg.ReadConf(filename)
			if tc.invalid {
				if err == nil || !strings.Contains(err.Error(), "logRotationPeriod") {
					t.Fatalf("expected logRotationPeriod error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LogRotationPeriod != tc.want {
				t.Fatalf("rotation period = %q, want %q", cfg.LogRotationPeriod, tc.want)
			}
		})
	}
}

func TestExistingDurationUnitsRemainCompatible(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(filename, []byte(`{"shutdownTimeout":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := cfg.ReadConf(filename); err != nil {
		t.Fatal(err)
	}
	if cfg.ShutdownTimeout.Duration != 2*time.Nanosecond {
		t.Fatalf("existing numeric duration units changed: %v", cfg.ShutdownTimeout.Duration)
	}
}

func TestLogRetentionDaysConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		want    int
		invalid bool
	}{
		{name: "default", json: `{}`, want: 30},
		{name: "disabled", json: `{"logRetentionDays":0}`, want: 0},
		{name: "one day", json: `{"logRetentionDays":1}`, want: 1},
		{name: "configured", json: `{"logRetentionDays":7}`, want: 7},
		{name: "duration boundary", json: `{"logRetentionDays":106751}`, want: 106751},
		{name: "negative", json: `{"logRetentionDays":-1}`, invalid: true},
		{name: "null", json: `{"logRetentionDays":null}`, invalid: true},
		{name: "string", json: `{"logRetentionDays":"7"}`, invalid: true},
		{name: "fraction", json: `{"logRetentionDays":1.5}`, invalid: true},
		{name: "decimal", json: `{"logRetentionDays":1.0}`, invalid: true},
		{name: "boolean", json: `{"logRetentionDays":true}`, invalid: true},
		{name: "array", json: `{"logRetentionDays":[]}`, invalid: true},
		{name: "object", json: `{"logRetentionDays":{}}`, invalid: true},
		{name: "duration overflow", json: `{"logRetentionDays":106752}`, invalid: true},
		{name: "integer overflow", json: `{"logRetentionDays":9223372036854775808}`, invalid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(filename, []byte(tc.json), 0o600); err != nil {
				t.Fatal(err)
			}
			var cfg Config
			err := cfg.ReadConf(filename)
			if tc.invalid {
				if err == nil || !strings.Contains(err.Error(), "logRetentionDays") {
					t.Fatalf("expected logRetentionDays error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LogRetentionDays != tc.want {
				t.Fatalf("retention days = %d, want %d", cfg.LogRetentionDays, tc.want)
			}
		})
	}
}

func TestLogRetentionDaysReloadDefaults(t *testing.T) {
	for _, data := range []string{`{"logRetentionDays":7}`, `{"logRetentionDays":0}`} {
		t.Run(data, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			var cfg Config
			if err := cfg.ReadConf(filename); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filename, []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := cfg.ReadConf(filename); err != nil {
				t.Fatal(err)
			}
			if cfg.LogRetentionDays != 30 {
				t.Fatalf("omitted retention days after reload = %d, want 30", cfg.LogRetentionDays)
			}
		})
	}
}
