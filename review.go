package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

var (
	validCustomExerciseTypes = []string{
		"weight_reps",
		"reps_only",
		"bodyweight_reps",
		"bodyweight_assisted_reps",
		"duration",
		"weight_duration",
		"distance_duration",
		"short_distance_weight",
	}
	validEquipmentCategories = []string{
		"none",
		"barbell",
		"dumbbell",
		"kettlebell",
		"machine",
		"plate",
		"resistance_band",
		"suspension",
		"other",
	}
	validMuscleGroups = []string{
		"abdominals",
		"shoulders",
		"biceps",
		"triceps",
		"forearms",
		"quadriceps",
		"hamstrings",
		"calves",
		"glutes",
		"abductors",
		"adductors",
		"lats",
		"upper_back",
		"traps",
		"lower_back",
		"chest",
		"cardio",
		"neck",
		"full_body",
		"other",
	}
)

type reviewUI struct {
	reader *bufio.Reader
	out    io.Writer
}

type reviewSummary struct {
	Reviewed    int
	Resolved    int
	Skipped     int
	Custom      int
	Deferred    int
	QuitEarly   bool
	Unresolved  int
	TemplateUse int
}

func newReviewUI(in io.Reader, out io.Writer) *reviewUI {
	return &reviewUI{
		reader: bufio.NewReader(in),
		out:    out,
	}
}

func runExercisesReview(ctx context.Context, cfg runtimeConfig, args []string) error {
	fs := flagSet("exercises review")
	var (
		refresh bool
		mapPath string
	)
	fs.BoolVar(&refresh, "refresh", false, "Refresh cached exercise templates")
	fs.StringVar(&mapPath, "map", cfg.exerciseMapPath(), "Path to exercise map file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	mapFile, err := loadExerciseMap(mapPath)
	if err != nil {
		return err
	}
	if len(mapFile.Exercises) == 0 {
		return fmt.Errorf("exercise map %s is empty; run `strong2hevy exercises resolve` first", mapPath)
	}

	templates, err := loadTemplatesForReview(ctx, cfg, refresh)
	if err != nil {
		return err
	}
	templateIndex := buildTemplateIndex(templates)

	var unresolved []*exerciseMapping
	for i := range mapFile.Exercises {
		if mapFile.Exercises[i].NeedsReview {
			unresolved = append(unresolved, &mapFile.Exercises[i])
		}
	}
	if len(unresolved) == 0 {
		fmt.Fprintln(os.Stdout, "No exercises need review.")
		return nil
	}
	if len(templates) == 0 {
		fmt.Fprintln(os.Stdout, "Warning: no cached or fetched Hevy templates available. You can still pick existing suggestions, skip entries, or create custom exercises, but ad hoc search is unavailable.")
	}

	ui := newReviewUI(os.Stdin, os.Stdout)
	fmt.Fprintf(ui.out, "Reviewing %d exercise mappings in %s\n", len(unresolved), mapPath)
	fmt.Fprintln(ui.out, "Actions: number=choose suggestion, /query=search templates, id <template_id>=choose exact template, s=skip, c=custom, n=next unresolved, q=quit and save.")

	summary := reviewSummary{Unresolved: len(unresolved)}
	for i, entry := range unresolved {
		result, err := reviewExerciseEntry(ui, entry, i+1, len(unresolved), templates, templateIndex)
		if err != nil {
			return err
		}
		summary.Reviewed++
		switch result {
		case "template":
			summary.Resolved++
			summary.TemplateUse++
			summary.Unresolved--
			if err := writeExerciseMap(mapPath, mapFile); err != nil {
				return err
			}
		case "skip":
			summary.Resolved++
			summary.Skipped++
			summary.Unresolved--
			if err := writeExerciseMap(mapPath, mapFile); err != nil {
				return err
			}
		case "custom":
			summary.Resolved++
			summary.Custom++
			summary.Unresolved--
			if err := writeExerciseMap(mapPath, mapFile); err != nil {
				return err
			}
		case "next":
			summary.Deferred++
		case "quit":
			summary.QuitEarly = true
			printReviewSummary(ui.out, summary)
			return nil
		}
	}

	printReviewSummary(ui.out, summary)
	return nil
}

func loadTemplatesForReview(ctx context.Context, cfg runtimeConfig, refresh bool) ([]hevyExerciseRef, error) {
	if !refresh {
		cache, err := loadTemplateCache(cfg.templateCachePath())
		if err == nil && len(cache.Templates) > 0 {
			return cache.Templates, nil
		}
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		if refresh {
			return nil, errors.New("missing Hevy API key; pass --api-key or set HEVY_API_KEY to refresh exercise templates")
		}
		return nil, nil
	}
	return fetchTemplates(ctx, cfg, newHevyClient(cfg.APIKey), refresh)
}

func reviewExerciseEntry(
	ui *reviewUI,
	entry *exerciseMapping,
	position int,
	total int,
	templates []hevyExerciseRef,
	templateIndex map[string]hevyExerciseRef,
) (string, error) {
	suggestions := entry.Suggestions
	if len(suggestions) == 0 && len(templates) > 0 {
		suggestions = findBestSuggestions(entry.StrongName, templates, 8)
	}

	for {
		printExerciseReview(ui.out, entry, position, total, suggestions)
		input, err := ui.prompt("> ")
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(ui.out)
				return "quit", nil
			}
			return "", err
		}

		switch {
		case input == "" || strings.EqualFold(input, "n") || strings.EqualFold(input, "next"):
			return "next", nil
		case strings.EqualFold(input, "q") || strings.EqualFold(input, "quit"):
			return "quit", nil
		case strings.EqualFold(input, "s") || strings.EqualFold(input, "skip"):
			applySkipMapping(entry)
			return "skip", nil
		case strings.EqualFold(input, "c") || strings.EqualFold(input, "custom"):
			custom, err := promptCustomMapping(ui, entry)
			if err != nil {
				return "", err
			}
			if custom == nil {
				continue
			}
			applyCustomMapping(entry, *custom)
			return "custom", nil
		case strings.EqualFold(input, "?") || strings.EqualFold(input, "help"):
			printReviewHelp(ui.out)
			continue
		case strings.HasPrefix(input, "/"):
			query := strings.TrimSpace(strings.TrimPrefix(input, "/"))
			if query == "" {
				query = entry.StrongName
			}
			if len(templates) == 0 {
				fmt.Fprintln(ui.out, "No template catalog available. Run `strong2hevy exercises resolve --refresh` or supply an API key, then try again.")
				continue
			}
			suggestions = findBestSuggestions(query, templates, 10)
			if len(suggestions) == 0 {
				fmt.Fprintf(ui.out, "No template suggestions found for %q\n", query)
			}
			continue
		case strings.HasPrefix(strings.ToLower(input), "id "):
			templateID := strings.TrimSpace(input[3:])
			template, ok := templateIndex[templateID]
			if !ok {
				fmt.Fprintf(ui.out, "Unknown template id %q\n", templateID)
				continue
			}
			applyTemplateSelection(entry, templateSuggestion{
				TemplateID:         template.ID,
				Title:              template.Title,
				Type:               template.Type,
				PrimaryMuscleGroup: template.PrimaryMuscleGroup,
				IsCustom:           template.IsCustom,
			})
			return "template", nil
		default:
			index, err := strconv.Atoi(input)
			if err != nil {
				fmt.Fprintf(ui.out, "Unrecognized action %q. Enter ? for help.\n", input)
				continue
			}
			if index < 1 || index > len(suggestions) {
				fmt.Fprintf(ui.out, "Choice %d is out of range.\n", index)
				continue
			}
			applyTemplateSelection(entry, suggestions[index-1])
			return "template", nil
		}
	}
}

func printExerciseReview(out io.Writer, entry *exerciseMapping, position, total int, suggestions []templateSuggestion) {
	fmt.Fprintln(out)
	fmt.Fprintf(out, "[%d/%d] %s\n", position, total, entry.StrongName)
	if entry.Action != "" {
		fmt.Fprintf(out, "Current action: %s", entry.Action)
		if entry.TemplateID != "" {
			fmt.Fprintf(out, " (%s)", entry.TemplateID)
		}
		fmt.Fprintln(out)
	}
	if entry.Custom != nil && entry.Custom.Title != "" {
		fmt.Fprintf(out, "Custom draft: title=%q type=%q equipment=%q muscle=%q\n", entry.Custom.Title, entry.Custom.ExerciseType, entry.Custom.EquipmentCategory, entry.Custom.MuscleGroup)
	}
	if len(suggestions) == 0 {
		fmt.Fprintln(out, "No suggestions available.")
		return
	}
	fmt.Fprintln(out, "Suggestions:")
	for i, suggestion := range suggestions {
		customLabel := ""
		if suggestion.IsCustom {
			customLabel = ", custom"
		}
		fmt.Fprintf(out, "  %d. %s [%s, %s%s] score=%.3f id=%s\n",
			i+1,
			suggestion.Title,
			emptyAsUnknown(suggestion.Type),
			emptyAsUnknown(suggestion.PrimaryMuscleGroup),
			customLabel,
			suggestion.Score,
			suggestion.TemplateID,
		)
	}
}

func printReviewHelp(out io.Writer) {
	fmt.Fprintln(out, "number: choose one of the current suggestions")
	fmt.Fprintln(out, "/query: search the Hevy template catalog and replace the suggestion list")
	fmt.Fprintln(out, "id <template_id>: choose a specific template id")
	fmt.Fprintln(out, "s: mark this exercise as skip and clear needs_review")
	fmt.Fprintln(out, "c: create a custom exercise mapping")
	fmt.Fprintln(out, "n or empty line: leave this entry unresolved for now")
	fmt.Fprintln(out, "q: quit the review session and keep progress already saved")
}

func printReviewSummary(out io.Writer, summary reviewSummary) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Review summary:")
	fmt.Fprintf(out, "  reviewed: %d\n", summary.Reviewed)
	fmt.Fprintf(out, "  resolved with template: %d\n", summary.TemplateUse)
	fmt.Fprintf(out, "  resolved as skip: %d\n", summary.Skipped)
	fmt.Fprintf(out, "  resolved as custom: %d\n", summary.Custom)
	fmt.Fprintf(out, "  left unresolved: %d\n", summary.Unresolved)
	if summary.Deferred > 0 {
		fmt.Fprintf(out, "  deferred this session: %d\n", summary.Deferred)
	}
	if summary.QuitEarly {
		fmt.Fprintln(out, "  session ended early: yes")
	}
}

func (ui *reviewUI) prompt(label string) (string, error) {
	fmt.Fprint(ui.out, label)
	line, err := ui.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if errors.Is(err, io.EOF) && line == "" {
		return "", io.EOF
	}
	return line, nil
}

func promptCustomMapping(ui *reviewUI, entry *exerciseMapping) (*customExerciseMapping, error) {
	existing := customExerciseMapping{Title: entry.StrongName}
	if entry.Custom != nil {
		existing = *entry.Custom
		if existing.Title == "" {
			existing.Title = entry.StrongName
		}
	}

	fmt.Fprintln(ui.out, "Custom exercise setup. Type `cancel` at any prompt to abort.")
	fmt.Fprintf(ui.out, "Valid exercise types: %s\n", strings.Join(validCustomExerciseTypes, ", "))
	fmt.Fprintf(ui.out, "Valid equipment categories: %s\n", strings.Join(validEquipmentCategories, ", "))
	fmt.Fprintf(ui.out, "Valid muscle groups: %s\n", strings.Join(validMuscleGroups, ", "))

	title, err := promptRequiredWithDefault(ui, "title", existing.Title)
	if err != nil {
		return nil, err
	}
	if title == "" {
		return nil, nil
	}
	exerciseType, err := promptEnumWithDefault(ui, "exercise_type", existing.ExerciseType, validCustomExerciseTypes)
	if err != nil {
		return nil, err
	}
	if exerciseType == "" {
		return nil, nil
	}
	equipmentCategory, err := promptEnumWithDefault(ui, "equipment_category", existing.EquipmentCategory, validEquipmentCategories)
	if err != nil {
		return nil, err
	}
	if equipmentCategory == "" {
		return nil, nil
	}
	muscleGroup, err := promptEnumWithDefault(ui, "muscle_group", existing.MuscleGroup, validMuscleGroups)
	if err != nil {
		return nil, err
	}
	if muscleGroup == "" {
		return nil, nil
	}

	otherDefault := strings.Join(existing.OtherMuscles, ",")
	otherMusclesLine, err := ui.prompt(fmt.Sprintf("other_muscles [%s]: ", otherDefault))
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(otherMusclesLine, "cancel") {
		return nil, nil
	}
	if otherMusclesLine == "" {
		otherMusclesLine = otherDefault
	}
	otherMuscles := parseCSVList(otherMusclesLine)
	for _, muscle := range otherMuscles {
		if !containsString(validMuscleGroups, muscle) {
			fmt.Fprintf(ui.out, "Invalid other_muscles value %q\n", muscle)
			return nil, nil
		}
	}

	return &customExerciseMapping{
		Title:             title,
		ExerciseType:      exerciseType,
		EquipmentCategory: equipmentCategory,
		MuscleGroup:       muscleGroup,
		OtherMuscles:      otherMuscles,
	}, nil
}

func promptRequiredWithDefault(ui *reviewUI, field, defaultValue string) (string, error) {
	for {
		value, err := ui.prompt(fmt.Sprintf("%s [%s]: ", field, defaultValue))
		if err != nil {
			return "", err
		}
		if strings.EqualFold(value, "cancel") {
			return "", nil
		}
		if value == "" {
			value = defaultValue
		}
		if strings.TrimSpace(value) == "" {
			fmt.Fprintf(ui.out, "%s is required.\n", field)
			continue
		}
		return strings.TrimSpace(value), nil
	}
}

func promptEnumWithDefault(ui *reviewUI, field, defaultValue string, allowed []string) (string, error) {
	for {
		value, err := ui.prompt(fmt.Sprintf("%s [%s]: ", field, defaultValue))
		if err != nil {
			return "", err
		}
		if strings.EqualFold(value, "cancel") {
			return "", nil
		}
		if value == "" {
			value = defaultValue
		}
		value = strings.TrimSpace(value)
		if !containsString(allowed, value) {
			fmt.Fprintf(ui.out, "Invalid %s %q\n", field, value)
			continue
		}
		return value, nil
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func parseCSVList(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func applyTemplateSelection(entry *exerciseMapping, suggestion templateSuggestion) {
	entry.Action = "use-template"
	entry.NeedsReview = false
	entry.TemplateID = suggestion.TemplateID
	entry.HevyTitle = suggestion.Title
	entry.Custom = nil
}

func applySkipMapping(entry *exerciseMapping) {
	entry.Action = "skip"
	entry.NeedsReview = false
	entry.TemplateID = ""
	entry.HevyTitle = ""
}

func applyCustomMapping(entry *exerciseMapping, custom customExerciseMapping) {
	entry.Action = "create-custom"
	entry.NeedsReview = false
	entry.TemplateID = ""
	entry.HevyTitle = ""
	entry.Custom = &custom
}

func emptyAsUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}
