package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var errNoImportableExercises = errors.New("no importable exercises after mapping")

func exerciseMapPointers(file *exerciseMapFile) map[string]*exerciseMapping {
	out := make(map[string]*exerciseMapping, len(file.Exercises))
	for i := range file.Exercises {
		entry := &file.Exercises[i]
		out[entry.StrongName] = entry
	}
	return out
}

func ensureTemplateID(
	ctx context.Context,
	client *hevyClient,
	templates []hevyExerciseRef,
	entry *exerciseMapping,
	dryRun bool,
) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("exercise mapping entry is nil")
	}
	if entry.NeedsReview {
		return "", fmt.Errorf("exercise %q still needs review in exercise map", entry.StrongName)
	}
	switch entry.Action {
	case "use-template":
		if strings.TrimSpace(entry.TemplateID) == "" {
			return "", fmt.Errorf("exercise %q uses template action but template_id is empty", entry.StrongName)
		}
		return entry.TemplateID, nil
	case "skip":
		return "", nil
	case "create-custom":
		if strings.TrimSpace(entry.TemplateID) != "" {
			return entry.TemplateID, nil
		}
		if entry.Custom == nil {
			return "", fmt.Errorf("exercise %q create-custom action requires custom metadata", entry.StrongName)
		}
		if entry.Custom.Title == "" || entry.Custom.ExerciseType == "" || entry.Custom.EquipmentCategory == "" || entry.Custom.MuscleGroup == "" {
			return "", fmt.Errorf("exercise %q create-custom action is missing required metadata", entry.StrongName)
		}
		for _, template := range templates {
			if template.IsCustom && normalizeTitle(template.Title) == normalizeTitle(entry.Custom.Title) {
				entry.TemplateID = template.ID
				entry.HevyTitle = template.Title
				return template.ID, nil
			}
		}
		if dryRun {
			return "dry-run-custom-" + normalizeTitle(entry.StrongName), nil
		}
		templateID, err := client.CreateCustomExercise(ctx, hevyCustomExerciseRequest{
			Exercise: customExercisePayload{
				Title:             entry.Custom.Title,
				ExerciseType:      entry.Custom.ExerciseType,
				EquipmentCategory: entry.Custom.EquipmentCategory,
				MuscleGroup:       entry.Custom.MuscleGroup,
				OtherMuscles:      entry.Custom.OtherMuscles,
			},
		})
		if err != nil {
			return "", fmt.Errorf("create custom exercise for %q: %w", entry.StrongName, err)
		}
		entry.TemplateID = templateID
		entry.HevyTitle = entry.Custom.Title
		return templateID, nil
	default:
		return "", fmt.Errorf("exercise %q has unsupported action %q", entry.StrongName, entry.Action)
	}
}

func buildWorkoutRequest(
	ctx context.Context,
	workout strongWorkout,
	cfg runtimeConfig,
	client *hevyClient,
	mapFile *exerciseMapFile,
	templates []hevyExerciseRef,
	visibility string,
	dryRun bool,
) (hevyWorkoutRequest, []string, error) {
	isPrivate, err := visibilityIsPrivate(visibility)
	if err != nil {
		return hevyWorkoutRequest{}, nil, err
	}
	exercises, skipped, err := buildExercisePayloads(ctx, cfg, client, mapFile, templates, workout.ExerciseList, dryRun)
	if err != nil {
		return hevyWorkoutRequest{}, nil, err
	}
	if len(exercises) == 0 {
		return hevyWorkoutRequest{}, skipped, fmt.Errorf("%w: workout %q on %s", errNoImportableExercises, workout.WorkoutName, workout.StartRaw)
	}
	duration, err := parseStrongDuration(workout.DurationRaw)
	if err != nil {
		return hevyWorkoutRequest{}, nil, fmt.Errorf("parse duration for workout %q: %w", workout.WorkoutName, err)
	}
	end := workout.Start.Add(duration)
	request := hevyWorkoutRequest{
		Workout: hevyWorkoutBody{
			Title:     workout.WorkoutName,
			StartTime: workout.Start.UTC().Format(time.RFC3339),
			EndTime:   end.UTC().Format(time.RFC3339),
			IsPrivate: isPrivate,
			Exercises: exercises,
		},
	}
	return request, skipped, nil
}

func buildRoutineRequest(
	ctx context.Context,
	workout strongWorkout,
	cfg runtimeConfig,
	client *hevyClient,
	mapFile *exerciseMapFile,
	templates []hevyExerciseRef,
	folderID *int,
	dryRun bool,
) (hevyRoutineRequest, []string, error) {
	exercises, skipped, err := buildExercisePayloads(ctx, cfg, client, mapFile, templates, workout.ExerciseList, dryRun)
	if err != nil {
		return hevyRoutineRequest{}, nil, err
	}
	if len(exercises) == 0 {
		return hevyRoutineRequest{}, skipped, fmt.Errorf("%w: routine %q", errNoImportableExercises, workout.WorkoutName)
	}
	return hevyRoutineRequest{
		Routine: hevyRoutineBody{
			Title:     workout.WorkoutName,
			FolderID:  folderID,
			Exercises: exercises,
		},
	}, skipped, nil
}

func buildExercisePayloads(
	ctx context.Context,
	cfg runtimeConfig,
	client *hevyClient,
	mapFile *exerciseMapFile,
	templates []hevyExerciseRef,
	exercises []strongExercise,
	dryRun bool,
) ([]hevyExercisePayload, []string, error) {
	pointers := exerciseMapPointers(mapFile)
	var skipped []string
	out := make([]hevyExercisePayload, 0, len(exercises))
	for _, exercise := range exercises {
		entry, ok := pointers[exercise.Name]
		if !ok {
			return nil, nil, fmt.Errorf("exercise %q is missing from exercise map", exercise.Name)
		}
		templateID, err := ensureTemplateID(ctx, client, templates, entry, dryRun)
		if err != nil {
			return nil, nil, err
		}
		if templateID == "" && entry.Action == "skip" {
			skipped = append(skipped, exercise.Name)
			continue
		}
		payload := hevyExercisePayload{
			ExerciseTemplateID: templateID,
			Sets:               make([]hevySetPayload, 0, len(exercise.Sets)),
		}
		for _, set := range exercise.Sets {
			converted, err := convertSet(cfg, set)
			if err != nil {
				return nil, nil, fmt.Errorf("convert set for %q: %w", exercise.Name, err)
			}
			payload.Sets = append(payload.Sets, converted)
		}
		out = append(out, payload)
	}
	return out, skipped, nil
}

func convertSet(cfg runtimeConfig, set strongSet) (hevySetPayload, error) {
	payload := hevySetPayload{
		Type: "normal",
	}
	if isWarmup(set.SetOrder) {
		payload.Type = "warmup"
	}
	if set.Weight > 0 {
		weight := weightToKG(set.Weight, cfg.WeightUnit)
		payload.WeightKG = &weight
	}
	if set.Reps != nil {
		reps := *set.Reps
		payload.Reps = &reps
	}
	if set.Distance > 0 {
		if strings.TrimSpace(cfg.DistanceUnit) == "" {
			return hevySetPayload{}, fmt.Errorf("distance set requires distance_unit configuration")
		}
		meters, err := distanceToMeters(set.Distance, cfg.DistanceUnit)
		if err != nil {
			return hevySetPayload{}, err
		}
		payload.DistanceMeters = &meters
	}
	if set.Seconds != nil {
		seconds := *set.Seconds
		payload.DurationSeconds = &seconds
	}
	if set.RPE != nil {
		valid := []float64{6, 7, 7.5, 8, 8.5, 9, 9.5, 10}
		if !slices.Contains(valid, *set.RPE) {
			return hevySetPayload{}, fmt.Errorf("unsupported rpe %.2f", *set.RPE)
		}
		rpe := *set.RPE
		payload.RPE = &rpe
	}
	return payload, nil
}
