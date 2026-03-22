package main

import (
	"fmt"
	"sort"
	"strings"
)

func buildRoutinePlans(workouts []strongWorkout) []routinePlan {
	byName := map[string][]strongWorkout{}
	for _, workout := range workouts {
		byName[workout.WorkoutName] = append(byName[workout.WorkoutName], workout)
	}

	var plans []routinePlan
	for workoutName, instances := range byName {
		if len(instances) < 2 {
			continue
		}
		signatures := map[string][]strongWorkout{}
		for _, workout := range instances {
			signatures[workoutSignature(workout)] = append(signatures[workoutSignature(workout)], workout)
		}

		var dominantKey string
		var dominant []strongWorkout
		for key, signatureWorkouts := range signatures {
			if len(signatureWorkouts) > len(dominant) {
				dominantKey = key
				dominant = signatureWorkouts
				continue
			}
			if len(signatureWorkouts) == len(dominant) && key < dominantKey {
				dominantKey = key
				dominant = signatureWorkouts
			}
		}
		sort.Slice(dominant, func(i, j int) bool {
			return dominant[i].Start.After(dominant[j].Start)
		})
		representative := dominant[0]
		plan := routinePlan{
			WorkoutName:         workoutName,
			Occurrences:         len(instances),
			DominantOccurrences: len(dominant),
			Stability:           roundFloat(float64(len(dominant)) / float64(len(instances))),
			Suggested:           len(instances) >= 3 && float64(len(dominant))/float64(len(instances)) >= 0.6,
			Selected:            false,
			Representative: representativeWorkout{
				SourceDate: representative.StartRaw,
				Duration:   representative.DurationRaw,
				Exercises:  summarizeRoutineExercises(representative),
			},
		}
		plans = append(plans, plan)
	}

	sort.Slice(plans, func(i, j int) bool {
		if plans[i].Occurrences == plans[j].Occurrences {
			return plans[i].WorkoutName < plans[j].WorkoutName
		}
		return plans[i].Occurrences > plans[j].Occurrences
	})
	return plans
}

func summarizeRoutineExercises(workout strongWorkout) []routinePlanExerciseRef {
	out := make([]routinePlanExerciseRef, 0, len(workout.ExerciseList))
	for _, exercise := range workout.ExerciseList {
		ref := routinePlanExerciseRef{
			StrongName: exercise.Name,
			SetCount:   len(exercise.Sets),
		}
		for _, set := range exercise.Sets {
			if isWarmup(set.SetOrder) {
				ref.WarmupCount++
			}
			if set.Seconds != nil {
				ref.TimedSets++
			}
			if set.Distance > 0 {
				ref.DistanceSets++
			}
			if set.RPE != nil {
				ref.RPESets++
			}
		}
		out = append(out, ref)
	}
	return out
}

func workoutSignature(workout strongWorkout) string {
	parts := make([]string, 0, len(workout.ExerciseList))
	for _, exercise := range workout.ExerciseList {
		setParts := make([]string, 0, len(exercise.Sets))
		for _, set := range exercise.Sets {
			setParts = append(setParts, setShape(set))
		}
		parts = append(parts, fmt.Sprintf("%s:%s", normalizeTitle(exercise.Name), strings.Join(setParts, ",")))
	}
	return strings.Join(parts, "|")
}

func setShape(set strongSet) string {
	setType := "n"
	if isWarmup(set.SetOrder) {
		setType = "w"
	}
	metric := "x"
	switch {
	case set.Distance > 0 && set.Seconds != nil:
		metric = "ds"
	case set.Seconds != nil && set.Weight > 0:
		metric = "ws"
	case set.Seconds != nil:
		metric = "s"
	case set.Distance > 0:
		metric = "d"
	case set.Reps != nil && set.Weight > 0:
		metric = "wr"
	case set.Reps != nil:
		metric = "r"
	case set.Weight > 0:
		metric = "w"
	}
	if set.RPE != nil {
		metric += "rpe"
	}
	return setType + metric
}

func findWorkoutByDateAndName(workouts []strongWorkout, date, name string) (strongWorkout, bool) {
	for _, workout := range workouts {
		if workout.StartRaw == date && workout.WorkoutName == name {
			return workout, true
		}
	}
	return strongWorkout{}, false
}
