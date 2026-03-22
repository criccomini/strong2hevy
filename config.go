package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath        = ".strong2hevy/config.yaml"
	defaultStateDir          = ".strong2hevy"
	defaultExerciseMapPath   = ".strong2hevy/exercise-map.yaml"
	defaultRoutinePlanPath   = ".strong2hevy/routines.yaml"
	defaultImportStatePath   = ".strong2hevy/import-state.json"
	defaultTemplateCachePath = ".strong2hevy/exercise-templates-cache.json"
)

type runtimeConfig struct {
	InputPath         string `yaml:"input"`
	APIKey            string `yaml:"api_key"`
	ConfigPath        string `yaml:"-"`
	Format            string `yaml:"format"`
	StateDir          string `yaml:"state_dir"`
	Timezone          string `yaml:"timezone"`
	WeightUnit        string `yaml:"weight_unit"`
	DistanceUnit      string `yaml:"distance_unit"`
	DefaultVisibility string `yaml:"default_visibility"`
}

type exerciseMapFile struct {
	Exercises []exerciseMapping `yaml:"exercises"`
}

type exerciseMapping struct {
	StrongName  string                 `yaml:"strong_name"`
	Action      string                 `yaml:"action"`
	NeedsReview bool                   `yaml:"needs_review,omitempty"`
	TemplateID  string                 `yaml:"template_id,omitempty"`
	HevyTitle   string                 `yaml:"hevy_title,omitempty"`
	Suggestions []templateSuggestion   `yaml:"suggestions,omitempty"`
	Custom      *customExerciseMapping `yaml:"custom,omitempty"`
}

type customExerciseMapping struct {
	Title             string   `yaml:"title"`
	ExerciseType      string   `yaml:"exercise_type"`
	EquipmentCategory string   `yaml:"equipment_category"`
	MuscleGroup       string   `yaml:"muscle_group"`
	OtherMuscles      []string `yaml:"other_muscles,omitempty"`
}

type templateSuggestion struct {
	TemplateID         string  `yaml:"template_id"`
	Title              string  `yaml:"title"`
	Type               string  `yaml:"type,omitempty"`
	PrimaryMuscleGroup string  `yaml:"primary_muscle_group,omitempty"`
	IsCustom           bool    `yaml:"is_custom,omitempty"`
	Score              float64 `yaml:"score,omitempty"`
}

type routinePlanFile struct {
	Routines []routinePlan `yaml:"routines"`
}

type routinePlan struct {
	WorkoutName         string                `yaml:"workout_name"`
	Occurrences         int                   `yaml:"occurrences"`
	DominantOccurrences int                   `yaml:"dominant_occurrences"`
	Stability           float64               `yaml:"stability"`
	Suggested           bool                  `yaml:"suggested"`
	Selected            bool                  `yaml:"selected"`
	Representative      representativeWorkout `yaml:"representative"`
}

type representativeWorkout struct {
	SourceDate string                   `yaml:"source_date"`
	Duration   string                   `yaml:"duration"`
	Exercises  []routinePlanExerciseRef `yaml:"exercises"`
}

type routinePlanExerciseRef struct {
	StrongName   string `yaml:"strong_name"`
	SetCount     int    `yaml:"set_count"`
	WarmupCount  int    `yaml:"warmup_count"`
	TimedSets    int    `yaml:"timed_sets,omitempty"`
	DistanceSets int    `yaml:"distance_sets,omitempty"`
	RPESets      int    `yaml:"rpe_sets,omitempty"`
}

type importStateFile struct {
	Workouts map[string]importStateEntry `json:"workouts"`
}

type importStateEntry struct {
	HevyWorkoutID string `json:"hevy_workout_id"`
	ImportedAt    string `json:"imported_at"`
	SourceDate    string `json:"source_date"`
	WorkoutName   string `json:"workout_name"`
}

type templateCacheFile struct {
	FetchedAt string            `json:"fetched_at"`
	Templates []hevyExerciseRef `json:"templates"`
}

func defaultRuntimeConfig() runtimeConfig {
	return runtimeConfig{
		InputPath:         filepath.Clean("data/strong_workouts.csv"),
		ConfigPath:        defaultConfigPath,
		Format:            "table",
		StateDir:          defaultStateDir,
		Timezone:          "Local",
		WeightUnit:        "lb",
		DistanceUnit:      "",
		DefaultVisibility: "private",
	}
}

func loadRuntimeConfig(globals globalOptions) (runtimeConfig, error) {
	cfg := defaultRuntimeConfig()

	if globals.ConfigPath != "" {
		cfg.ConfigPath = globals.ConfigPath
	}

	if err := mergeConfigFile(&cfg, cfg.ConfigPath); err != nil {
		return cfg, err
	}

	if globals.InputPath != "" {
		cfg.InputPath = globals.InputPath
	}
	if globals.APIKey != "" {
		cfg.APIKey = globals.APIKey
	}
	if globals.Format != "" {
		cfg.Format = strings.ToLower(globals.Format)
	}
	if cfg.APIKey == "" {
		cfg.APIKey = strings.TrimSpace(os.Getenv("HEVY_API_KEY"))
	}
	if cfg.Format == "" {
		cfg.Format = "table"
	}
	if cfg.StateDir == "" {
		cfg.StateDir = defaultStateDir
	}
	return cfg, nil
}

func mergeConfigFile(cfg *runtimeConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	return nil
}

func writeConfigFile(path string, cfg runtimeConfig, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config file %s already exists; pass --force to overwrite", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat config %s: %w", path, err)
		}
	}

	toWrite := runtimeConfig{
		InputPath:         cfg.InputPath,
		APIKey:            "",
		Format:            cfg.Format,
		StateDir:          cfg.StateDir,
		Timezone:          cfg.Timezone,
		WeightUnit:        cfg.WeightUnit,
		DistanceUnit:      cfg.DistanceUnit,
		DefaultVisibility: cfg.DefaultVisibility,
	}
	data, err := yaml.Marshal(toWrite)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return writeFileAtomic(path, data)
}

func (cfg runtimeConfig) exerciseMapPath() string {
	return filepath.Join(cfg.StateDir, filepath.Base(defaultExerciseMapPath))
}

func (cfg runtimeConfig) routinePlanPath() string {
	return filepath.Join(cfg.StateDir, filepath.Base(defaultRoutinePlanPath))
}

func (cfg runtimeConfig) importStatePath() string {
	return filepath.Join(cfg.StateDir, filepath.Base(defaultImportStatePath))
}

func (cfg runtimeConfig) templateCachePath() string {
	return filepath.Join(cfg.StateDir, filepath.Base(defaultTemplateCachePath))
}

func ensureStateDir(cfg runtimeConfig) error {
	return os.MkdirAll(cfg.StateDir, 0o755)
}

func loadExerciseMap(path string) (exerciseMapFile, error) {
	var file exerciseMapFile
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return file, nil
		}
		return file, fmt.Errorf("read exercise map %s: %w", path, err)
	}
	if len(data) == 0 {
		return file, nil
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("parse exercise map %s: %w", path, err)
	}
	sort.Slice(file.Exercises, func(i, j int) bool {
		return file.Exercises[i].StrongName < file.Exercises[j].StrongName
	})
	return file, nil
}

func writeExerciseMap(path string, file exerciseMapFile) error {
	sort.Slice(file.Exercises, func(i, j int) bool {
		return file.Exercises[i].StrongName < file.Exercises[j].StrongName
	})
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal exercise map: %w", err)
	}
	return writeFileAtomic(path, data)
}

func loadRoutinePlan(path string) (routinePlanFile, error) {
	var file routinePlanFile
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return file, nil
		}
		return file, fmt.Errorf("read routine plan %s: %w", path, err)
	}
	if len(data) == 0 {
		return file, nil
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("parse routine plan %s: %w", path, err)
	}
	return file, nil
}

func writeRoutinePlan(path string, file routinePlanFile) error {
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal routine plan: %w", err)
	}
	return writeFileAtomic(path, data)
}

func loadImportState(path string) (importStateFile, error) {
	var file importStateFile
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return importStateFile{Workouts: map[string]importStateEntry{}}, nil
		}
		return file, fmt.Errorf("read import state %s: %w", path, err)
	}
	if len(data) == 0 {
		return importStateFile{Workouts: map[string]importStateEntry{}}, nil
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("parse import state %s: %w", path, err)
	}
	if file.Workouts == nil {
		file.Workouts = map[string]importStateEntry{}
	}
	return file, nil
}

func writeImportState(path string, file importStateFile) error {
	if file.Workouts == nil {
		file.Workouts = map[string]importStateEntry{}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal import state: %w", err)
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data)
}

func loadTemplateCache(path string) (templateCacheFile, error) {
	var file templateCacheFile
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return file, nil
		}
		return file, fmt.Errorf("read template cache %s: %w", path, err)
	}
	if len(data) == 0 {
		return file, nil
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("parse template cache %s: %w", path, err)
	}
	return file, nil
}

func writeTemplateCache(path string, file templateCacheFile) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal template cache: %w", err)
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func exerciseMapIndex(file exerciseMapFile) map[string]exerciseMapping {
	out := make(map[string]exerciseMapping, len(file.Exercises))
	for _, entry := range file.Exercises {
		out[entry.StrongName] = entry
	}
	return out
}
