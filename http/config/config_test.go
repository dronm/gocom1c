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
	if cfg.ShutdownTimeout.Duration != 2*time.Second {
		t.Fatalf("existing numeric duration units changed: %v", cfg.ShutdownTimeout.Duration)
	}
}
