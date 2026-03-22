package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

func runInit(_ context.Context, cfg runtimeConfig, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		force        bool
		inputPath    string
		format       string
		stateDir     string
		timezoneName string
		weightUnit   string
		distanceUnit string
		visibility   string
	)
	fs.BoolVar(&force, "force", false, "Overwrite an existing config file")
	fs.StringVar(&inputPath, "input", cfg.InputPath, "Path to Strong CSV export")
	fs.StringVar(&format, "format", cfg.Format, "Default output format: table or json")
	fs.StringVar(&stateDir, "state-dir", cfg.StateDir, "Directory for generated state files")
	fs.StringVar(&timezoneName, "timezone", cfg.Timezone, "Default timezone for Strong timestamps")
	fs.StringVar(&weightUnit, "weight-unit", cfg.WeightUnit, "Default weight unit: lb or kg")
	fs.StringVar(&distanceUnit, "distance-unit", cfg.DistanceUnit, "Default distance unit: mi, km, or m")
	fs.StringVar(&visibility, "visibility", cfg.DefaultVisibility, "Default workout visibility: private or public")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	cfg.InputPath = inputPath
	cfg.Format = strings.ToLower(strings.TrimSpace(format))
	cfg.StateDir = stateDir
	cfg.Timezone = timezoneName
	cfg.WeightUnit = strings.ToLower(strings.TrimSpace(weightUnit))
	cfg.DistanceUnit = strings.ToLower(strings.TrimSpace(distanceUnit))
	cfg.DefaultVisibility = strings.ToLower(strings.TrimSpace(visibility))
	cfg.APIKey = ""

	if cfg.Format != "table" && cfg.Format != "json" {
		return fmt.Errorf("unsupported format %q", cfg.Format)
	}
	if cfg.WeightUnit != "lb" && cfg.WeightUnit != "kg" {
		return fmt.Errorf("unsupported weight unit %q", cfg.WeightUnit)
	}
	if cfg.DistanceUnit != "" && cfg.DistanceUnit != "mi" && cfg.DistanceUnit != "km" && cfg.DistanceUnit != "m" {
		return fmt.Errorf("unsupported distance unit %q", cfg.DistanceUnit)
	}
	if _, err := visibilityIsPrivate(cfg.DefaultVisibility); err != nil {
		return err
	}
	if _, err := loadLocation(cfg.Timezone); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.StateDir) == "" {
		return errors.New("state-dir cannot be empty")
	}

	if err := writeConfigFile(cfg.ConfigPath, cfg, force); err != nil {
		return err
	}

	summary := struct {
		ConfigPath string        `json:"config_path"`
		Config     runtimeConfig `json:"config"`
	}{
		ConfigPath: cfg.ConfigPath,
		Config:     cfg,
	}
	if cfg.Format == "json" {
		return outputJSON(summary)
	}
	w := mustTableWriter()
	fmt.Fprintf(w, "Wrote config\t%s\n", cfg.ConfigPath)
	fmt.Fprintf(w, "Input\t%s\n", cfg.InputPath)
	fmt.Fprintf(w, "Format\t%s\n", cfg.Format)
	fmt.Fprintf(w, "State dir\t%s\n", cfg.StateDir)
	fmt.Fprintf(w, "Timezone\t%s\n", cfg.Timezone)
	fmt.Fprintf(w, "Weight unit\t%s\n", cfg.WeightUnit)
	fmt.Fprintf(w, "Distance unit\t%s\n", cfg.DistanceUnit)
	fmt.Fprintf(w, "Default visibility\t%s\n", cfg.DefaultVisibility)
	return w.Flush()
}

func runDoctor(ctx context.Context, cfg runtimeConfig, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var refresh bool
	fs.BoolVar(&refresh, "refresh", false, "Refresh cached exercise templates")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	type doctorCheck struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Details string `json:"details"`
	}
	report := struct {
		Checks []doctorCheck `json:"checks"`
	}{}
	failed := false

	location, err := loadLocation(cfg.Timezone)
	if err != nil {
		report.Checks = append(report.Checks, doctorCheck{Name: "timezone", Status: "fail", Details: err.Error()})
		failed = true
	} else {
		report.Checks = append(report.Checks, doctorCheck{Name: "timezone", Status: "ok", Details: location.String()})
	}

	data, dataErr := loadStrongData(cfg.InputPath, locationOrLocal(location))
	if dataErr != nil {
		report.Checks = append(report.Checks, doctorCheck{Name: "input", Status: "fail", Details: dataErr.Error()})
		failed = true
	} else {
		report.Checks = append(report.Checks, doctorCheck{Name: "input", Status: "ok", Details: fmt.Sprintf("%d workouts, %d rows", len(data.Workouts), len(data.Rows))})
		if containsDistanceRows(data) && strings.TrimSpace(cfg.DistanceUnit) == "" {
			report.Checks = append(report.Checks, doctorCheck{Name: "distance_unit", Status: "warn", Details: "distance rows exist but distance_unit is not configured"})
		} else if containsDistanceRows(data) {
			report.Checks = append(report.Checks, doctorCheck{Name: "distance_unit", Status: "ok", Details: cfg.DistanceUnit})
		}
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		report.Checks = append(report.Checks, doctorCheck{Name: "api_key", Status: "fail", Details: "missing Hevy API key"})
		failed = true
	} else {
		report.Checks = append(report.Checks, doctorCheck{Name: "api_key", Status: "ok", Details: "present"})
		client := newHevyClient(cfg.APIKey)
		userInfo, err := client.GetUserInfo(ctx)
		if err != nil {
			report.Checks = append(report.Checks, doctorCheck{Name: "user_info", Status: "fail", Details: err.Error()})
			failed = true
		} else {
			report.Checks = append(report.Checks, doctorCheck{Name: "user_info", Status: "ok", Details: fmt.Sprintf("%s (%s)", userInfo.Username, userInfo.Email)})
		}
		templates, err := fetchTemplates(ctx, cfg, client, refresh)
		if err != nil {
			report.Checks = append(report.Checks, doctorCheck{Name: "exercise_templates", Status: "fail", Details: err.Error()})
			failed = true
		} else {
			report.Checks = append(report.Checks, doctorCheck{Name: "exercise_templates", Status: "ok", Details: fmt.Sprintf("%d templates cached", len(templates))})
		}
	}

	if cfg.Format == "json" {
		if err := outputJSON(report); err != nil {
			return err
		}
	} else {
		w := mustTableWriter()
		fmt.Fprintln(w, "CHECK\tSTATUS\tDETAILS")
		for _, check := range report.Checks {
			fmt.Fprintf(w, "%s\t%s\t%s\n", check.Name, check.Status, check.Details)
		}
		w.Flush()
	}

	if failed {
		return errors.New("doctor reported failures")
	}
	return nil
}

func runAnalyze(_ context.Context, cfg runtimeConfig, args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	location, err := loadLocation(cfg.Timezone)
	if err != nil {
		return err
	}
	data, err := loadStrongData(cfg.InputPath, location)
	if err != nil {
		return err
	}
	report := buildAnalysisReport(cfg.InputPath, data)
	if cfg.Format == "json" {
		return outputJSON(report)
	}
	w := mustTableWriter()
	fmt.Fprintf(w, "Input\t%s\n", report.InputPath)
	fmt.Fprintf(w, "Rows\t%d\n", report.RowCount)
	fmt.Fprintf(w, "Workouts\t%d\n", report.WorkoutCount)
	fmt.Fprintf(w, "Exercise names\t%d\n", report.ExerciseNameCount)
	fmt.Fprintf(w, "Workout names\t%d\n", report.WorkoutNameCount)
	fmt.Fprintf(w, "Date range\t%s -> %s\n", report.DateRangeStart, report.DateRangeEnd)
	fmt.Fprintf(w, "Warmup sets\t%d\n", report.WarmupSets)
	fmt.Fprintf(w, "Work sets\t%d\n", report.WorkSets)
	fmt.Fprintf(w, "Timed sets\t%d\n", report.TimedSetCount)
	fmt.Fprintf(w, "Distance sets\t%d\n", report.DistanceSetCount)
	fmt.Fprintf(w, "RPE sets\t%d\n", report.RPESetCount)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Top Workout Names\tCount")
	for _, item := range report.WorkoutNameFrequency {
		fmt.Fprintf(w, "%s\t%d\n", item.Name, item.Count)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Top Exercise Names\tCount")
	for _, item := range report.ExerciseNameFrequency {
		fmt.Fprintf(w, "%s\t%d\n", item.Name, item.Count)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Routine Candidates\tOccurrences\tDominant\tStability")
	for _, item := range report.RoutineCandidates {
		fmt.Fprintf(w, "%s\t%d\t%d\t%.3f\n", item.WorkoutName, item.Occurrences, item.DominantOccurrences, item.Stability)
	}
	return w.Flush()
}

func runExercises(ctx context.Context, cfg runtimeConfig, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected subcommand: search or resolve")
	}
	switch args[0] {
	case "search":
		return runExercisesSearch(ctx, cfg, args[1:])
	case "resolve":
		return runExercisesResolve(ctx, cfg, args[1:])
	default:
		return fmt.Errorf("unknown exercises subcommand %q", args[0])
	}
}

func runExercisesSearch(ctx context.Context, cfg runtimeConfig, args []string) error {
	fs := flag.NewFlagSet("exercises search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var refresh bool
	fs.BoolVar(&refresh, "refresh", false, "Refresh cached exercise templates")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) == 0 {
		return fmt.Errorf("exercises search requires a query")
	}
	if err := requireAPIKey(cfg); err != nil {
		return err
	}
	query := strings.Join(fs.Args(), " ")
	client := newHevyClient(cfg.APIKey)
	templates, err := fetchTemplates(ctx, cfg, client, refresh)
	if err != nil {
		return err
	}
	type searchResult struct {
		TemplateID         string  `json:"template_id"`
		Title              string  `json:"title"`
		Type               string  `json:"type"`
		PrimaryMuscleGroup string  `json:"primary_muscle_group"`
		IsCustom           bool    `json:"is_custom"`
		Score              float64 `json:"score"`
	}
	var results []searchResult
	for _, template := range templates {
		score := suggestionScore(query, template.Title)
		if score < 0.3 {
			continue
		}
		results = append(results, searchResult{
			TemplateID:         template.ID,
			Title:              template.Title,
			Type:               template.Type,
			PrimaryMuscleGroup: template.PrimaryMuscleGroup,
			IsCustom:           template.IsCustom,
			Score:              roundFloat(score),
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].IsCustom == results[j].IsCustom {
				return results[i].Title < results[j].Title
			}
			return !results[i].IsCustom && results[j].IsCustom
		}
		return results[i].Score > results[j].Score
	})
	if cfg.Format == "json" {
		return outputJSON(results)
	}
	w := mustTableWriter()
	fmt.Fprintln(w, "ID\tTITLE\tTYPE\tPRIMARY MUSCLE\tCUSTOM\tSCORE")
	for _, result := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%.3f\n", result.TemplateID, result.Title, result.Type, result.PrimaryMuscleGroup, result.IsCustom, result.Score)
	}
	return w.Flush()
}

func runExercisesResolve(ctx context.Context, cfg runtimeConfig, args []string) error {
	fs := flag.NewFlagSet("exercises resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var refresh bool
	var mapPath string
	fs.BoolVar(&refresh, "refresh", false, "Refresh cached exercise templates")
	fs.StringVar(&mapPath, "map", cfg.exerciseMapPath(), "Path to exercise map file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireAPIKey(cfg); err != nil {
		return err
	}
	location, err := loadLocation(cfg.Timezone)
	if err != nil {
		return err
	}
	data, err := loadStrongData(cfg.InputPath, location)
	if err != nil {
		return err
	}
	client := newHevyClient(cfg.APIKey)
	templates, err := fetchTemplates(ctx, cfg, client, refresh)
	if err != nil {
		return err
	}
	existing, err := loadExerciseMap(mapPath)
	if err != nil {
		return err
	}
	existingIndex := exerciseMapIndex(existing)
	templateIndex := buildTemplateIndex(templates)

	out := exerciseMapFile{Exercises: make([]exerciseMapping, 0, len(data.ExerciseNames))}
	autoResolved := 0
	needsReview := 0
	reused := 0
	for _, name := range data.ExerciseNames {
		if existingEntry, ok := existingIndex[name]; ok {
			if existingEntry.Action == "use-template" {
				if template, ok := templateIndex[existingEntry.TemplateID]; ok {
					existingEntry.HevyTitle = template.Title
				}
			}
			out.Exercises = append(out.Exercises, existingEntry)
			reused++
			if existingEntry.NeedsReview {
				needsReview++
			}
			continue
		}
		if template, ok := chooseExactTemplate(name, templates); ok {
			out.Exercises = append(out.Exercises, exerciseMapping{
				StrongName: name,
				Action:     "use-template",
				TemplateID: template.ID,
				HevyTitle:  template.Title,
			})
			autoResolved++
			continue
		}
		out.Exercises = append(out.Exercises, exerciseMapping{
			StrongName:  name,
			Action:      "skip",
			NeedsReview: true,
			Suggestions: findBestSuggestions(name, templates, 5),
			Custom: &customExerciseMapping{
				Title:             name,
				ExerciseType:      "",
				EquipmentCategory: "",
				MuscleGroup:       "",
			},
		})
		needsReview++
	}

	if err := ensureStateDir(cfg); err != nil {
		return err
	}
	if err := writeExerciseMap(mapPath, out); err != nil {
		return err
	}

	summary := struct {
		MapPath       string `json:"map_path"`
		Total         int    `json:"total"`
		AutoResolved  int    `json:"auto_resolved"`
		Reused        int    `json:"reused"`
		NeedsReview   int    `json:"needs_review"`
		TemplateCount int    `json:"template_count"`
	}{
		MapPath:       mapPath,
		Total:         len(out.Exercises),
		AutoResolved:  autoResolved,
		Reused:        reused,
		NeedsReview:   needsReview,
		TemplateCount: len(templates),
	}
	if cfg.Format == "json" {
		return outputJSON(summary)
	}
	w := mustTableWriter()
	fmt.Fprintf(w, "Exercise map\t%s\n", summary.MapPath)
	fmt.Fprintf(w, "Strong exercises\t%d\n", summary.Total)
	fmt.Fprintf(w, "Auto resolved\t%d\n", summary.AutoResolved)
	fmt.Fprintf(w, "Reused existing\t%d\n", summary.Reused)
	fmt.Fprintf(w, "Needs review\t%d\n", summary.NeedsReview)
	return w.Flush()
}

func runRoutines(ctx context.Context, cfg runtimeConfig, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected subcommand: plan or apply")
	}
	switch args[0] {
	case "plan":
		return runRoutinesPlan(ctx, cfg, args[1:])
	case "apply":
		return runRoutinesApply(ctx, cfg, args[1:])
	default:
		return fmt.Errorf("unknown routines subcommand %q", args[0])
	}
}

func runRoutinesPlan(_ context.Context, cfg runtimeConfig, args []string) error {
	fs := flag.NewFlagSet("routines plan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var outPath string
	fs.StringVar(&outPath, "out", cfg.routinePlanPath(), "Path to routine plan file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	location, err := loadLocation(cfg.Timezone)
	if err != nil {
		return err
	}
	data, err := loadStrongData(cfg.InputPath, location)
	if err != nil {
		return err
	}
	planFile := routinePlanFile{Routines: buildRoutinePlans(data.Workouts)}
	if err := ensureStateDir(cfg); err != nil {
		return err
	}
	if err := writeRoutinePlan(outPath, planFile); err != nil {
		return err
	}
	suggested := 0
	for _, plan := range planFile.Routines {
		if plan.Suggested {
			suggested++
		}
	}
	summary := struct {
		PlanPath   string `json:"plan_path"`
		Candidates int    `json:"candidates"`
		Suggested  int    `json:"suggested"`
	}{
		PlanPath:   outPath,
		Candidates: len(planFile.Routines),
		Suggested:  suggested,
	}
	if cfg.Format == "json" {
		return outputJSON(summary)
	}
	w := mustTableWriter()
	fmt.Fprintf(w, "Routine plan\t%s\n", summary.PlanPath)
	fmt.Fprintf(w, "Candidates\t%d\n", summary.Candidates)
	fmt.Fprintf(w, "Suggested\t%d\n", summary.Suggested)
	return w.Flush()
}

func runRoutinesApply(ctx context.Context, cfg runtimeConfig, args []string) error {
	fs := flag.NewFlagSet("routines apply", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		planPath       string
		mapPath        string
		folderArg      string
		updateExisting bool
		dryRun         bool
		refresh        bool
	)
	fs.StringVar(&planPath, "plan", cfg.routinePlanPath(), "Path to routine plan file")
	fs.StringVar(&mapPath, "map", cfg.exerciseMapPath(), "Path to exercise map file")
	fs.StringVar(&folderArg, "folder", "", "Routine folder name or id")
	fs.BoolVar(&updateExisting, "update-existing", false, "Update existing routines matched by exact title")
	fs.BoolVar(&dryRun, "dry-run", false, "Build requests without sending them")
	fs.BoolVar(&refresh, "refresh", false, "Refresh cached exercise templates")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireAPIKey(cfg); err != nil {
		return err
	}
	location, err := loadLocation(cfg.Timezone)
	if err != nil {
		return err
	}
	data, err := loadStrongData(cfg.InputPath, location)
	if err != nil {
		return err
	}
	planFile, err := loadRoutinePlan(planPath)
	if err != nil {
		return err
	}
	mapFile, err := loadExerciseMap(mapPath)
	if err != nil {
		return err
	}
	client := newHevyClient(cfg.APIKey)
	templates, err := fetchTemplates(ctx, cfg, client, refresh)
	if err != nil {
		return err
	}
	folderID, err := parseFolderArg(ctx, client, folderArg)
	if err != nil {
		return err
	}
	existingRoutines, err := client.ListAllRoutines(ctx)
	if err != nil {
		return err
	}
	existingByTitle := map[string]hevyRoutineSummary{}
	for _, routine := range existingRoutines {
		existingByTitle[strings.ToLower(routine.Title)] = routine
	}

	type applySummary struct {
		Created int      `json:"created"`
		Updated int      `json:"updated"`
		Skipped int      `json:"skipped"`
		Errors  []string `json:"errors,omitempty"`
	}
	summary := applySummary{}
	for _, plan := range planFile.Routines {
		if !plan.Selected {
			continue
		}
		workout, ok := findWorkoutByDateAndName(data.Workouts, plan.Representative.SourceDate, plan.WorkoutName)
		if !ok {
			summary.Errors = append(summary.Errors, fmt.Sprintf("representative workout missing for %q on %s", plan.WorkoutName, plan.Representative.SourceDate))
			continue
		}
		request, _, err := buildRoutineRequest(ctx, workout, cfg, client, &mapFile, templates, folderID, dryRun)
		if err != nil {
			summary.Errors = append(summary.Errors, err.Error())
			continue
		}
		if existing, ok := existingByTitle[strings.ToLower(plan.WorkoutName)]; ok {
			if !updateExisting {
				summary.Skipped++
				continue
			}
			if dryRun {
				summary.Updated++
				continue
			}
			if _, err := client.UpdateRoutine(ctx, existing.ID, request); err != nil {
				summary.Errors = append(summary.Errors, err.Error())
				continue
			}
			summary.Updated++
			continue
		}
		if dryRun {
			summary.Created++
			continue
		}
		if _, err := client.CreateRoutine(ctx, request); err != nil {
			summary.Errors = append(summary.Errors, err.Error())
			continue
		}
		summary.Created++
	}

	if !dryRun {
		if err := writeExerciseMap(mapPath, mapFile); err != nil {
			return err
		}
	}
	if cfg.Format == "json" {
		if err := outputJSON(summary); err != nil {
			return err
		}
	} else {
		w := mustTableWriter()
		fmt.Fprintf(w, "Created\t%d\n", summary.Created)
		fmt.Fprintf(w, "Updated\t%d\n", summary.Updated)
		fmt.Fprintf(w, "Skipped\t%d\n", summary.Skipped)
		fmt.Fprintf(w, "Errors\t%d\n", len(summary.Errors))
		for _, line := range summary.Errors {
			fmt.Fprintf(w, "Error\t%s\n", line)
		}
		w.Flush()
	}
	if len(summary.Errors) > 0 {
		return errors.New("routines apply completed with errors")
	}
	return nil
}

func runWorkouts(ctx context.Context, cfg runtimeConfig, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected subcommand: import")
	}
	switch args[0] {
	case "import":
		return runWorkoutsImport(ctx, cfg, args[1:])
	default:
		return fmt.Errorf("unknown workouts subcommand %q", args[0])
	}
}

func runWorkoutsImport(ctx context.Context, cfg runtimeConfig, args []string) error {
	fs := flag.NewFlagSet("workouts import", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		fromDate     string
		toDate       string
		mapPath      string
		statePath    string
		visibility   string
		timezoneName string
		distanceUnit string
		dryRun       bool
		refresh      bool
	)
	fs.StringVar(&fromDate, "from", "", "Import workouts on or after YYYY-MM-DD")
	fs.StringVar(&toDate, "to", "", "Import workouts on or before YYYY-MM-DD")
	fs.StringVar(&mapPath, "map", cfg.exerciseMapPath(), "Path to exercise map file")
	fs.StringVar(&statePath, "state", cfg.importStatePath(), "Path to import state file")
	fs.StringVar(&visibility, "visibility", cfg.DefaultVisibility, "Workout visibility: private or public")
	fs.StringVar(&timezoneName, "timezone", cfg.Timezone, "IANA timezone for Strong timestamps")
	fs.StringVar(&distanceUnit, "distance-unit", cfg.DistanceUnit, "Distance unit for Strong distances: mi, km, or m")
	fs.BoolVar(&dryRun, "dry-run", false, "Build requests without sending them")
	fs.BoolVar(&refresh, "refresh", false, "Refresh cached exercise templates")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg.Timezone = timezoneName
	cfg.DistanceUnit = distanceUnit
	if err := requireAPIKey(cfg); err != nil {
		return err
	}
	location, err := loadLocation(cfg.Timezone)
	if err != nil {
		return err
	}
	data, err := loadStrongData(cfg.InputPath, location)
	if err != nil {
		return err
	}
	if containsDistanceRows(data) && strings.TrimSpace(cfg.DistanceUnit) == "" {
		return errors.New("distance rows exist but distance_unit is not configured; pass --distance-unit or set it in config")
	}
	mapFile, err := loadExerciseMap(mapPath)
	if err != nil {
		return err
	}
	state, err := loadImportState(statePath)
	if err != nil {
		return err
	}
	client := newHevyClient(cfg.APIKey)
	templates, err := fetchTemplates(ctx, cfg, client, refresh)
	if err != nil {
		return err
	}
	from, err := parseDateFilter(fromDate, location)
	if err != nil {
		return err
	}
	to, err := parseDateFilter(toDate, location)
	if err != nil {
		return err
	}

	type importSummary struct {
		Imported int      `json:"imported"`
		Skipped  int      `json:"skipped"`
		Errors   []string `json:"errors,omitempty"`
	}
	summary := importSummary{}

	for _, workout := range data.Workouts {
		if !matchesDateFilter(workout.Start, from, to) {
			continue
		}
		if _, ok := state.Workouts[workout.Hash]; ok {
			summary.Skipped++
			continue
		}
		request, skippedExercises, err := buildWorkoutRequest(ctx, workout, cfg, client, &mapFile, templates, visibility, dryRun)
		if err != nil {
			summary.Errors = append(summary.Errors, err.Error())
			continue
		}
		if dryRun {
			_ = skippedExercises
			summary.Imported++
			continue
		}
		response, err := client.CreateWorkout(ctx, request)
		if err != nil {
			summary.Errors = append(summary.Errors, err.Error())
			continue
		}
		state.Workouts[workout.Hash] = importStateEntry{
			HevyWorkoutID: formatAnyID(response),
			ImportedAt:    time.Now().UTC().Format(time.RFC3339),
			SourceDate:    workout.StartRaw,
			WorkoutName:   workout.WorkoutName,
		}
		if err := writeImportState(statePath, state); err != nil {
			return err
		}
		summary.Imported++
	}

	if !dryRun {
		if err := writeExerciseMap(mapPath, mapFile); err != nil {
			return err
		}
	}
	if cfg.Format == "json" {
		if err := outputJSON(summary); err != nil {
			return err
		}
	} else {
		w := mustTableWriter()
		fmt.Fprintf(w, "Imported\t%d\n", summary.Imported)
		fmt.Fprintf(w, "Skipped\t%d\n", summary.Skipped)
		fmt.Fprintf(w, "Errors\t%d\n", len(summary.Errors))
		for _, line := range summary.Errors {
			fmt.Fprintf(w, "Error\t%s\n", line)
		}
		w.Flush()
	}
	if len(summary.Errors) > 0 {
		return errors.New("workouts import completed with errors")
	}
	return nil
}

func matchesDateFilter(workoutTime, from, to time.Time) bool {
	if !from.IsZero() {
		if workoutTime.Before(from) {
			return false
		}
	}
	if !to.IsZero() {
		endOfDay := to.Add(24*time.Hour - time.Nanosecond)
		if workoutTime.After(endOfDay) {
			return false
		}
	}
	return true
}

func locationOrLocal(location *time.Location) *time.Location {
	if location == nil {
		return time.Local
	}
	return location
}
