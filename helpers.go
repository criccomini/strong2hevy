package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
)

func outputJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func mustTableWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
}

func loadLocation(name string) (*time.Location, error) {
	if strings.TrimSpace(name) == "" || strings.EqualFold(name, "Local") {
		return time.Local, nil
	}
	return time.LoadLocation(name)
}

func normalizeTitle(input string) string {
	var builder strings.Builder
	lastSpace := false
	for _, r := range strings.ToLower(strings.TrimSpace(input)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r) || strings.ContainsRune("-_()/&+’'", r):
			if !lastSpace && builder.Len() > 0 {
				builder.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func suggestionScore(query, candidate string) float64 {
	query = normalizeTitle(query)
	candidate = normalizeTitle(candidate)
	if query == "" || candidate == "" {
		return 0
	}
	if query == candidate {
		return 1
	}
	maxLen := max(len(query), len(candidate))
	distance := levenshtein(query, candidate)
	distanceScore := 1 - float64(distance)/float64(maxLen)
	tokenScore := tokenOverlapScore(query, candidate)
	score := (distanceScore * 0.65) + (tokenScore * 0.35)
	if strings.Contains(candidate, query) || strings.Contains(query, candidate) {
		score += 0.05
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func tokenOverlapScore(left, right string) float64 {
	leftTokens := strings.Fields(left)
	rightTokens := strings.Fields(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	leftSet := map[string]struct{}{}
	for _, token := range leftTokens {
		leftSet[token] = struct{}{}
	}
	matches := 0
	for _, token := range rightTokens {
		if _, ok := leftSet[token]; ok {
			matches++
		}
	}
	denominator := len(leftTokens)
	if len(rightTokens) > denominator {
		denominator = len(rightTokens)
	}
	return float64(matches) / float64(denominator)
}

func levenshtein(left, right string) int {
	if left == right {
		return 0
	}
	if len(left) == 0 {
		return len(right)
	}
	if len(right) == 0 {
		return len(left)
	}
	previous := make([]int, len(right)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(left); i++ {
		current := make([]int, len(right)+1)
		current[0] = i
		for j := 1; j <= len(right); j++ {
			cost := 0
			if left[i-1] != right[j-1] {
				cost = 1
			}
			current[j] = min(
				current[j-1]+1,
				previous[j]+1,
				previous[j-1]+cost,
			)
		}
		previous = current
	}
	return previous[len(right)]
}

func min(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}

func max(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value > best {
			best = value
		}
	}
	return best
}

func roundFloat(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func fetchTemplates(ctx context.Context, cfg runtimeConfig, client *hevyClient, refresh bool) ([]hevyExerciseRef, error) {
	cachePath := cfg.templateCachePath()
	if !refresh {
		cache, err := loadTemplateCache(cachePath)
		if err == nil && len(cache.Templates) > 0 {
			return cache.Templates, nil
		}
	}
	templates, err := client.ListAllExerciseTemplates(ctx)
	if err != nil {
		return nil, err
	}
	if err := ensureStateDir(cfg); err != nil {
		return nil, err
	}
	if err := writeTemplateCache(cachePath, templateCacheFile{
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Templates: templates,
	}); err != nil {
		return nil, err
	}
	return templates, nil
}

func requireAPIKey(cfg runtimeConfig) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return errors.New("missing Hevy API key; pass --api-key or set HEVY_API_KEY")
	}
	return nil
}

func weightToKG(weight float64, unit string) float64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "kg", "kgs":
		return roundFloat(weight)
	case "lb", "lbs":
		return roundFloat(weight * 0.45359237)
	default:
		return roundFloat(weight)
	}
}

func distanceToMeters(distance float64, unit string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "m", "meter", "meters":
		return int(math.Round(distance)), nil
	case "km", "kilometer", "kilometers":
		return int(math.Round(distance * 1000)), nil
	case "mi", "mile", "miles":
		return int(math.Round(distance * 1609.344)), nil
	default:
		return 0, fmt.Errorf("unsupported distance unit %q", unit)
	}
}

func findBestSuggestions(name string, templates []hevyExerciseRef, limit int) []templateSuggestion {
	suggestions := make([]templateSuggestion, 0, limit)
	for _, template := range templates {
		score := suggestionScore(name, template.Title)
		if score < 0.45 {
			continue
		}
		suggestions = append(suggestions, templateSuggestion{
			TemplateID:         template.ID,
			Title:              template.Title,
			Type:               template.Type,
			PrimaryMuscleGroup: template.PrimaryMuscleGroup,
			IsCustom:           template.IsCustom,
			Score:              roundFloat(score),
		})
	}
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Score == suggestions[j].Score {
			if suggestions[i].IsCustom == suggestions[j].IsCustom {
				return suggestions[i].Title < suggestions[j].Title
			}
			return !suggestions[i].IsCustom && suggestions[j].IsCustom
		}
		return suggestions[i].Score > suggestions[j].Score
	})
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}
	return suggestions
}

func chooseExactTemplate(name string, templates []hevyExerciseRef) (hevyExerciseRef, bool) {
	normalized := normalizeTitle(name)
	var exact []hevyExerciseRef
	for _, template := range templates {
		if normalizeTitle(template.Title) == normalized {
			exact = append(exact, template)
		}
	}
	if len(exact) == 0 {
		return hevyExerciseRef{}, false
	}
	sort.Slice(exact, func(i, j int) bool {
		if exact[i].IsCustom == exact[j].IsCustom {
			return exact[i].Title < exact[j].Title
		}
		return !exact[i].IsCustom && exact[j].IsCustom
	})
	return exact[0], true
}

func buildTemplateIndex(templates []hevyExerciseRef) map[string]hevyExerciseRef {
	out := make(map[string]hevyExerciseRef, len(templates))
	for _, template := range templates {
		out[template.ID] = template
	}
	return out
}

func parseDateFilter(value string, location *time.Location) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("2006-01-02", value, location)
}

func containsDistanceRows(data strongData) bool {
	for _, row := range data.Rows {
		if row.Distance > 0 {
			return true
		}
	}
	return false
}

func formatAnyID(response map[string]any) string {
	if id, ok := response["id"]; ok {
		return fmt.Sprint(id)
	}
	if workout, ok := response["workout"].(map[string]any); ok {
		if id, ok := workout["id"]; ok {
			return fmt.Sprint(id)
		}
	}
	return ""
}

func visibilityIsPrivate(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "private", "":
		return true, nil
	case "public":
		return false, nil
	default:
		return false, fmt.Errorf("unsupported visibility %q", value)
	}
}

func parseFolderArg(ctx context.Context, client *hevyClient, value string) (*int, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	if id, err := strconv.Atoi(value); err == nil {
		return &id, nil
	}
	folders, err := client.ListRoutineFolders(ctx)
	if err != nil {
		return nil, err
	}
	for _, folder := range folders {
		if strings.EqualFold(folder.Title, value) {
			id := folder.ID
			return &id, nil
		}
	}
	return nil, fmt.Errorf("routine folder %q not found", value)
}
