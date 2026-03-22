package main

import "testing"

func TestParseCustomExerciseID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{name: "plain uuid", body: "b459cba5-cd6d-463c-abd6-54f8eafcadcb", expected: "b459cba5-cd6d-463c-abd6-54f8eafcadcb"},
		{name: "json object", body: `{"id":"A05C064D"}`, expected: "A05C064D"},
		{name: "json string", body: `"4F5866F8"`, expected: "4F5866F8"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCustomExerciseID([]byte(test.body))
			if err != nil {
				t.Fatalf("parseCustomExerciseID returned error: %v", err)
			}
			if got != test.expected {
				t.Fatalf("parseCustomExerciseID(%q) = %q, want %q", test.body, got, test.expected)
			}
		})
	}
}
