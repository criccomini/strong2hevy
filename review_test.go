package main

import "testing"

func TestApplyTemplateSelection(t *testing.T) {
	t.Parallel()

	entry := &exerciseMapping{
		StrongName:  "Back Extension",
		Action:      "skip",
		NeedsReview: true,
		Custom: &customExerciseMapping{
			Title:             "Back Extension",
			ExerciseType:      "reps_only",
			EquipmentCategory: "other",
			MuscleGroup:       "lower_back",
		},
	}
	applyTemplateSelection(entry, templateSuggestion{
		TemplateID: "A05C064D",
		Title:      "Back Extension (Machine)",
	})

	if entry.Action != "use-template" {
		t.Fatalf("entry.Action = %q, want use-template", entry.Action)
	}
	if entry.NeedsReview {
		t.Fatal("entry.NeedsReview = true, want false")
	}
	if entry.TemplateID != "A05C064D" {
		t.Fatalf("entry.TemplateID = %q, want A05C064D", entry.TemplateID)
	}
	if entry.HevyTitle != "Back Extension (Machine)" {
		t.Fatalf("entry.HevyTitle = %q, want Back Extension (Machine)", entry.HevyTitle)
	}
	if entry.Custom != nil {
		t.Fatal("entry.Custom was not cleared")
	}
}

func TestApplyCustomMapping(t *testing.T) {
	t.Parallel()

	entry := &exerciseMapping{
		StrongName:  "Back Extension",
		NeedsReview: true,
		TemplateID:  "A05C064D",
		HevyTitle:   "Back Extension (Machine)",
	}
	applyCustomMapping(entry, customExerciseMapping{
		Title:             "Back Extension",
		ExerciseType:      "reps_only",
		EquipmentCategory: "machine",
		MuscleGroup:       "lower_back",
	})

	if entry.Action != "create-custom" {
		t.Fatalf("entry.Action = %q, want create-custom", entry.Action)
	}
	if entry.NeedsReview {
		t.Fatal("entry.NeedsReview = true, want false")
	}
	if entry.TemplateID != "" || entry.HevyTitle != "" {
		t.Fatalf("expected template fields to be cleared, got template_id=%q hevy_title=%q", entry.TemplateID, entry.HevyTitle)
	}
	if entry.Custom == nil || entry.Custom.ExerciseType != "reps_only" {
		t.Fatalf("entry.Custom = %#v, want populated custom mapping", entry.Custom)
	}
}

func TestParseCSVList(t *testing.T) {
	t.Parallel()

	got := parseCSVList(" lats,  biceps , , triceps ")
	if len(got) != 3 {
		t.Fatalf("len(parseCSVList(...)) = %d, want 3", len(got))
	}
	if got[0] != "lats" || got[1] != "biceps" || got[2] != "triceps" {
		t.Fatalf("parseCSVList returned %#v", got)
	}
}
