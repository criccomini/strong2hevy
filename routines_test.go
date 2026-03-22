package main

import (
	"testing"
	"time"
)

func TestBuildRoutinePlansUsesDominantMostRecentRepresentative(t *testing.T) {
	t.Parallel()

	workouts := []strongWorkout{
		newWorkoutForTest(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), "A", "Plan A", []strongExercise{
			{Name: "Bench Press (Barbell)", Sets: []strongSet{{SetOrder: "1"}, {SetOrder: "2"}}},
			{Name: "Squat (Barbell)", Sets: []strongSet{{SetOrder: "1"}}},
		}),
		newWorkoutForTest(time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC), "B", "Plan A", []strongExercise{
			{Name: "Bench Press (Barbell)", Sets: []strongSet{{SetOrder: "1"}, {SetOrder: "2"}}},
			{Name: "Squat (Barbell)", Sets: []strongSet{{SetOrder: "1"}}},
		}),
		newWorkoutForTest(time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC), "C", "Plan A", []strongExercise{
			{Name: "Bench Press (Barbell)", Sets: []strongSet{{SetOrder: "1"}}},
		}),
	}

	plans := buildRoutinePlans(workouts)
	if len(plans) != 1 {
		t.Fatalf("len(plans) = %d, want 1", len(plans))
	}
	if plans[0].DominantOccurrences != 2 {
		t.Fatalf("DominantOccurrences = %d, want 2", plans[0].DominantOccurrences)
	}
	if plans[0].Representative.SourceDate != "B" {
		t.Fatalf("Representative.SourceDate = %q, want B", plans[0].Representative.SourceDate)
	}
}

func newWorkoutForTest(start time.Time, sourceDate string, name string, exercises []strongExercise) strongWorkout {
	return strongWorkout{
		Start:        start,
		StartRaw:     sourceDate,
		WorkoutName:  name,
		DurationRaw:  "45m",
		ExerciseList: exercises,
	}
}
