package main

import "testing"

func TestConvertSet(t *testing.T) {
	t.Parallel()

	reps := 5
	seconds := 700
	rpe := 8.5
	payload, err := convertSet(runtimeConfig{
		WeightUnit:   "lb",
		DistanceUnit: "mi",
	}, strongSet{
		SetOrder: "W",
		Weight:   135,
		Reps:     &reps,
		Distance: 1,
		Seconds:  &seconds,
		RPE:      &rpe,
	})
	if err != nil {
		t.Fatalf("convertSet returned error: %v", err)
	}
	if payload.Type != "warmup" {
		t.Fatalf("payload.Type = %q, want warmup", payload.Type)
	}
	if payload.WeightKG == nil || *payload.WeightKG != 61.235 {
		t.Fatalf("payload.WeightKG = %v, want 61.235", payload.WeightKG)
	}
	if payload.DistanceMeters == nil || *payload.DistanceMeters != 1609 {
		t.Fatalf("payload.DistanceMeters = %v, want 1609", payload.DistanceMeters)
	}
	if payload.DurationSeconds == nil || *payload.DurationSeconds != 700 {
		t.Fatalf("payload.DurationSeconds = %v, want 700", payload.DurationSeconds)
	}
}

func TestChooseExactTemplatePrefersNonCustom(t *testing.T) {
	t.Parallel()

	templates := []hevyExerciseRef{
		{ID: "custom-1", Title: "Bench Press (Barbell)", IsCustom: true},
		{ID: "builtin-1", Title: "Bench Press (Barbell)", IsCustom: false},
	}
	got, ok := chooseExactTemplate("Bench Press (Barbell)", templates)
	if !ok {
		t.Fatal("expected exact match")
	}
	if got.ID != "builtin-1" {
		t.Fatalf("chooseExactTemplate returned %q, want builtin-1", got.ID)
	}
}
