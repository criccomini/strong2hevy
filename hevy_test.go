package main

import (
	"encoding/json"
	"testing"
)

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

func TestRoutineCreateRequestMarshalsNullFolderID(t *testing.T) {
	t.Parallel()

	request := hevyRoutineRequest{
		Routine: hevyRoutineBody{
			Title:    "Upper Body",
			FolderID: nil,
			Exercises: []hevyExercisePayload{
				{
					ExerciseTemplateID: "05293BCA",
					Sets: []hevySetPayload{
						{Type: "normal"},
					},
				},
			},
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	routine, ok := body["routine"].(map[string]any)
	if !ok {
		t.Fatalf("body[routine] = %T, want map[string]any", body["routine"])
	}
	value, ok := routine["folder_id"]
	if !ok {
		t.Fatalf("folder_id missing from create request JSON: %s", string(data))
	}
	if value != nil {
		t.Fatalf("folder_id = %#v, want null", value)
	}
}

func TestRoutineUpdateRequestOmitsFolderID(t *testing.T) {
	t.Parallel()

	folderID := 42
	request := hevyRoutineRequest{
		Routine: hevyRoutineBody{
			Title:    "Upper Body",
			FolderID: &folderID,
			Exercises: []hevyExercisePayload{
				{
					ExerciseTemplateID: "05293BCA",
					Sets: []hevySetPayload{
						{Type: "normal"},
					},
				},
			},
		},
	}

	data, err := json.Marshal(toUpdateRoutineRequest(request))
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	routine, ok := body["routine"].(map[string]any)
	if !ok {
		t.Fatalf("body[routine] = %T, want map[string]any", body["routine"])
	}
	if _, ok := routine["folder_id"]; ok {
		t.Fatalf("folder_id should be omitted from update request JSON: %s", string(data))
	}
}
