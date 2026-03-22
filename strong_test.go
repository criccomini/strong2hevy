package main

import (
	"testing"
	"time"
)

func TestParseStrongDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{name: "hours and minutes", input: "1h 17m", expected: time.Hour + 17*time.Minute},
		{name: "minutes only", input: "58m", expected: 58 * time.Minute},
		{name: "minutes and seconds", input: "6m 30s", expected: 6*time.Minute + 30*time.Second},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseStrongDuration(test.input)
			if err != nil {
				t.Fatalf("parseStrongDuration(%q) returned error: %v", test.input, err)
			}
			if got != test.expected {
				t.Fatalf("parseStrongDuration(%q) = %s, want %s", test.input, got, test.expected)
			}
		})
	}
}

func TestMatchesDateFilter(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC-8", -8*60*60)
	workoutTime := time.Date(2025, 3, 28, 13, 30, 0, 0, loc)
	from := time.Date(2025, 3, 28, 0, 0, 0, 0, loc)
	to := time.Date(2025, 3, 28, 0, 0, 0, 0, loc)

	if !matchesDateFilter(workoutTime, from, to) {
		t.Fatal("expected workout on filter boundary to match")
	}
}
