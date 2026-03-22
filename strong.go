package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type strongRow struct {
	Start        time.Time
	Date         string
	WorkoutName  string
	Duration     string
	ExerciseName string
	SetOrder     string
	Weight       float64
	Reps         *int
	Distance     float64
	Seconds      *int
	RPE          *float64
	Raw          map[string]string
}

type strongWorkout struct {
	Start        time.Time
	StartRaw     string
	WorkoutName  string
	DurationRaw  string
	Rows         []strongRow
	ExerciseList []strongExercise
	Hash         string
}

type strongExercise struct {
	Name string
	Sets []strongSet
}

type strongSet struct {
	SetOrder string
	Weight   float64
	Reps     *int
	Distance float64
	Seconds  *int
	RPE      *float64
}

type strongData struct {
	Rows             []strongRow
	Workouts         []strongWorkout
	ExerciseNames    []string
	WorkoutNameCount map[string]int
}

type analysisReport struct {
	InputPath             string             `json:"input_path"`
	RowCount              int                `json:"row_count"`
	WorkoutCount          int                `json:"workout_count"`
	ExerciseNameCount     int                `json:"exercise_name_count"`
	WorkoutNameCount      int                `json:"workout_name_count"`
	DateRangeStart        string             `json:"date_range_start"`
	DateRangeEnd          string             `json:"date_range_end"`
	WarmupSets            int                `json:"warmup_sets"`
	WorkSets              int                `json:"work_sets"`
	TimedSetCount         int                `json:"timed_set_count"`
	DistanceSetCount      int                `json:"distance_set_count"`
	RPESetCount           int                `json:"rpe_set_count"`
	WorkoutNameFrequency  []namedCount       `json:"workout_name_frequency"`
	ExerciseNameFrequency []namedCount       `json:"exercise_name_frequency"`
	RoutineCandidates     []routineCandidate `json:"routine_candidates"`
}

type namedCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type routineCandidate struct {
	WorkoutName         string  `json:"workout_name"`
	Occurrences         int     `json:"occurrences"`
	DominantOccurrences int     `json:"dominant_occurrences"`
	Stability           float64 `json:"stability"`
}

func loadStrongData(path string, location *time.Location) (strongData, error) {
	file, err := os.Open(path)
	if err != nil {
		return strongData{}, fmt.Errorf("open strong csv %s: %w", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return strongData{}, fmt.Errorf("read strong csv header: %w", err)
	}
	required := map[string]struct{}{
		"Date":          {},
		"Workout Name":  {},
		"Duration":      {},
		"Exercise Name": {},
		"Set Order":     {},
		"Weight":        {},
		"Reps":          {},
		"Distance":      {},
		"Seconds":       {},
		"RPE":           {},
	}
	index := map[string]int{}
	for i, column := range header {
		index[column] = i
	}
	for column := range required {
		if _, ok := index[column]; !ok {
			return strongData{}, fmt.Errorf("strong csv missing required column %q", column)
		}
	}

	var rows []strongRow
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return strongData{}, fmt.Errorf("read strong csv row: %w", err)
		}
		raw := make(map[string]string, len(header))
		for i, column := range header {
			if i < len(record) {
				raw[column] = record[i]
			} else {
				raw[column] = ""
			}
		}

		start, err := parseStrongTimestamp(raw["Date"], location)
		if err != nil {
			return strongData{}, fmt.Errorf("parse workout date %q: %w", raw["Date"], err)
		}
		row, err := parseStrongRow(raw, start)
		if err != nil {
			return strongData{}, err
		}
		rows = append(rows, row)
	}

	workouts, exerciseNames, workoutNameCount := buildStrongWorkouts(rows)
	return strongData{
		Rows:             rows,
		Workouts:         workouts,
		ExerciseNames:    exerciseNames,
		WorkoutNameCount: workoutNameCount,
	}, nil
}

func parseStrongRow(raw map[string]string, start time.Time) (strongRow, error) {
	weight, err := parseOptionalFloat(raw["Weight"])
	if err != nil {
		return strongRow{}, fmt.Errorf("parse weight for %q: %w", raw["Exercise Name"], err)
	}
	distance, err := parseOptionalFloat(raw["Distance"])
	if err != nil {
		return strongRow{}, fmt.Errorf("parse distance for %q: %w", raw["Exercise Name"], err)
	}
	reps, err := parseOptionalInt(raw["Reps"])
	if err != nil {
		return strongRow{}, fmt.Errorf("parse reps for %q: %w", raw["Exercise Name"], err)
	}
	seconds, err := parseOptionalInt(raw["Seconds"])
	if err != nil {
		return strongRow{}, fmt.Errorf("parse seconds for %q: %w", raw["Exercise Name"], err)
	}
	rpe, err := parseOptionalRPE(raw["RPE"])
	if err != nil {
		return strongRow{}, fmt.Errorf("parse rpe for %q: %w", raw["Exercise Name"], err)
	}

	return strongRow{
		Start:        start,
		Date:         start.Format("2006-01-02 15:04:05"),
		WorkoutName:  raw["Workout Name"],
		Duration:     raw["Duration"],
		ExerciseName: raw["Exercise Name"],
		SetOrder:     raw["Set Order"],
		Weight:       weight,
		Reps:         reps,
		Distance:     distance,
		Seconds:      seconds,
		RPE:          rpe,
		Raw:          raw,
	}, nil
}

func buildStrongWorkouts(rows []strongRow) ([]strongWorkout, []string, map[string]int) {
	type workoutKey struct {
		Date string
		Name string
	}
	workoutIndex := map[workoutKey]int{}
	exerciseIndexByWorkout := map[workoutKey]map[string]int{}
	workouts := make([]strongWorkout, 0, len(rows)/8)
	exerciseSet := map[string]struct{}{}
	workoutNameCount := map[string]int{}

	for _, row := range rows {
		key := workoutKey{Date: row.Date, Name: row.WorkoutName}
		idx, ok := workoutIndex[key]
		if !ok {
			workouts = append(workouts, strongWorkout{
				Start:       row.Start,
				StartRaw:    row.Date,
				WorkoutName: row.WorkoutName,
				DurationRaw: row.Duration,
			})
			idx = len(workouts) - 1
			workoutIndex[key] = idx
			exerciseIndexByWorkout[key] = map[string]int{}
			workoutNameCount[row.WorkoutName]++
		}

		workout := &workouts[idx]
		workout.Rows = append(workout.Rows, row)
		exerciseSet[row.ExerciseName] = struct{}{}

		exerciseIdx, ok := exerciseIndexByWorkout[key][row.ExerciseName]
		if !ok {
			workout.ExerciseList = append(workout.ExerciseList, strongExercise{Name: row.ExerciseName})
			exerciseIdx = len(workout.ExerciseList) - 1
			exerciseIndexByWorkout[key][row.ExerciseName] = exerciseIdx
		}
		workout.ExerciseList[exerciseIdx].Sets = append(workout.ExerciseList[exerciseIdx].Sets, strongSet{
			SetOrder: row.SetOrder,
			Weight:   row.Weight,
			Reps:     row.Reps,
			Distance: row.Distance,
			Seconds:  row.Seconds,
			RPE:      row.RPE,
		})
	}

	for i := range workouts {
		workouts[i].Hash = hashStrongWorkout(workouts[i])
	}

	exerciseNames := make([]string, 0, len(exerciseSet))
	for name := range exerciseSet {
		exerciseNames = append(exerciseNames, name)
	}
	sort.Strings(exerciseNames)

	sort.Slice(workouts, func(i, j int) bool {
		if workouts[i].Start.Equal(workouts[j].Start) {
			return workouts[i].WorkoutName < workouts[j].WorkoutName
		}
		return workouts[i].Start.Before(workouts[j].Start)
	})

	return workouts, exerciseNames, workoutNameCount
}

func hashStrongWorkout(workout strongWorkout) string {
	var builder strings.Builder
	builder.WriteString(workout.StartRaw)
	builder.WriteString("|")
	builder.WriteString(workout.WorkoutName)
	builder.WriteString("|")
	builder.WriteString(workout.DurationRaw)
	for _, row := range workout.Rows {
		builder.WriteString("|")
		builder.WriteString(strings.Join([]string{
			row.ExerciseName,
			row.SetOrder,
			row.Raw["Weight"],
			row.Raw["Reps"],
			row.Raw["Distance"],
			row.Raw["Seconds"],
			row.Raw["RPE"],
		}, ","))
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func buildAnalysisReport(inputPath string, data strongData) analysisReport {
	report := analysisReport{
		InputPath:         inputPath,
		RowCount:          len(data.Rows),
		WorkoutCount:      len(data.Workouts),
		ExerciseNameCount: len(data.ExerciseNames),
		WorkoutNameCount:  len(data.WorkoutNameCount),
	}
	if len(data.Workouts) > 0 {
		report.DateRangeStart = data.Workouts[0].StartRaw
		report.DateRangeEnd = data.Workouts[len(data.Workouts)-1].StartRaw
	}
	for _, row := range data.Rows {
		if isWarmup(row.SetOrder) {
			report.WarmupSets++
		} else {
			report.WorkSets++
		}
		if row.Seconds != nil {
			report.TimedSetCount++
		}
		if row.Distance > 0 {
			report.DistanceSetCount++
		}
		if row.RPE != nil {
			report.RPESetCount++
		}
	}
	report.WorkoutNameFrequency = topNamedCounts(data.WorkoutNameCount, 10)
	report.ExerciseNameFrequency = topNamedCounts(countExerciseNames(data.Rows), 10)
	report.RoutineCandidates = topRoutineCandidates(data.Workouts, 10)
	return report
}

func countExerciseNames(rows []strongRow) map[string]int {
	out := map[string]int{}
	for _, row := range rows {
		out[row.ExerciseName]++
	}
	return out
}

func topNamedCounts(input map[string]int, limit int) []namedCount {
	out := make([]namedCount, 0, len(input))
	for name, count := range input {
		out = append(out, namedCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Name < out[j].Name
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func topRoutineCandidates(workouts []strongWorkout, limit int) []routineCandidate {
	plans := buildRoutinePlans(workouts)
	out := make([]routineCandidate, 0, len(plans))
	for _, plan := range plans {
		out = append(out, routineCandidate{
			WorkoutName:         plan.WorkoutName,
			Occurrences:         plan.Occurrences,
			DominantOccurrences: plan.DominantOccurrences,
			Stability:           plan.Stability,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Occurrences == out[j].Occurrences {
			return out[i].WorkoutName < out[j].WorkoutName
		}
		return out[i].Occurrences > out[j].Occurrences
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func parseStrongTimestamp(value string, location *time.Location) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", value, location)
}

func parseStrongDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	var total time.Duration
	parts := strings.Fields(value)
	for _, part := range parts {
		if strings.HasSuffix(part, "h") {
			hours, err := strconv.Atoi(strings.TrimSuffix(part, "h"))
			if err != nil {
				return 0, err
			}
			total += time.Duration(hours) * time.Hour
			continue
		}
		if strings.HasSuffix(part, "m") {
			minutes, err := strconv.Atoi(strings.TrimSuffix(part, "m"))
			if err != nil {
				return 0, err
			}
			total += time.Duration(minutes) * time.Minute
			continue
		}
		if strings.HasSuffix(part, "s") {
			seconds, err := strconv.Atoi(strings.TrimSuffix(part, "s"))
			if err != nil {
				return 0, err
			}
			total += time.Duration(seconds) * time.Second
			continue
		}
		return 0, fmt.Errorf("unknown duration segment %q", part)
	}
	return total, nil
}

func parseOptionalFloat(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	if math.Abs(parsed) < 1e-9 {
		return 0, nil
	}
	return parsed, nil
}

func parseOptionalInt(value string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, err
	}
	if math.Abs(parsed) < 1e-9 {
		return nil, nil
	}
	out := int(math.Round(parsed))
	return &out, nil
}

func parseOptionalRPE(value string) (*float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, err
	}
	if math.Abs(parsed) < 1e-9 {
		return nil, nil
	}
	out := parsed
	return &out, nil
}

func isWarmup(setOrder string) bool {
	return strings.EqualFold(strings.TrimSpace(setOrder), "W")
}
