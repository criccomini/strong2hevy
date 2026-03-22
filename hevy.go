package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const hevyBaseURL = "https://api.hevyapp.com"

type hevyClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type hevyUserInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type hevyExerciseRef struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Type               string   `json:"type"`
	PrimaryMuscleGroup string   `json:"primary_muscle_group"`
	SecondaryMuscles   []string `json:"secondary_muscle_groups"`
	IsCustom           bool     `json:"is_custom"`
}

type hevyRoutineFolder struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type hevyRoutineSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type hevyWorkoutRequest struct {
	Workout hevyWorkoutBody `json:"workout"`
}

type hevyWorkoutBody struct {
	Title       string                `json:"title"`
	Description string                `json:"description,omitempty"`
	StartTime   string                `json:"start_time"`
	EndTime     string                `json:"end_time"`
	IsPrivate   bool                  `json:"is_private"`
	Exercises   []hevyExercisePayload `json:"exercises"`
}

type hevyRoutineRequest struct {
	Routine hevyRoutineBody `json:"routine"`
}

type hevyRoutineBody struct {
	Title     string                `json:"title"`
	FolderID  *int                  `json:"folder_id,omitempty"`
	Notes     string                `json:"notes,omitempty"`
	Exercises []hevyExercisePayload `json:"exercises"`
}

type hevyExercisePayload struct {
	ExerciseTemplateID string           `json:"exercise_template_id"`
	Notes              string           `json:"notes,omitempty"`
	Sets               []hevySetPayload `json:"sets"`
}

type hevySetPayload struct {
	Type            string   `json:"type"`
	WeightKG        *float64 `json:"weight_kg,omitempty"`
	Reps            *int     `json:"reps,omitempty"`
	DistanceMeters  *int     `json:"distance_meters,omitempty"`
	DurationSeconds *int     `json:"duration_seconds,omitempty"`
	RPE             *float64 `json:"rpe,omitempty"`
}

type hevyCustomExerciseRequest struct {
	Exercise customExercisePayload `json:"exercise"`
}

type customExercisePayload struct {
	Title             string   `json:"title"`
	ExerciseType      string   `json:"exercise_type"`
	EquipmentCategory string   `json:"equipment_category"`
	MuscleGroup       string   `json:"muscle_group"`
	OtherMuscles      []string `json:"other_muscles,omitempty"`
}

func newHevyClient(apiKey string) *hevyClient {
	return &hevyClient{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: hevyBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *hevyClient) GetUserInfo(ctx context.Context) (hevyUserInfo, error) {
	var response hevyUserInfo
	if err := c.do(ctx, http.MethodGet, "/v1/user/info", nil, &response); err != nil {
		return response, err
	}
	return response, nil
}

func (c *hevyClient) ListAllExerciseTemplates(ctx context.Context) ([]hevyExerciseRef, error) {
	var out []hevyExerciseRef
	page := 1
	for {
		var response struct {
			PageCount         int               `json:"page_count"`
			ExerciseTemplates []hevyExerciseRef `json:"exercise_templates"`
		}
		values := url.Values{}
		values.Set("page", strconv.Itoa(page))
		values.Set("pageSize", "100")
		if err := c.do(ctx, http.MethodGet, "/v1/exercise_templates?"+values.Encode(), nil, &response); err != nil {
			return nil, err
		}
		out = append(out, response.ExerciseTemplates...)
		if page >= response.PageCount || len(response.ExerciseTemplates) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (c *hevyClient) ListAllRoutines(ctx context.Context) ([]hevyRoutineSummary, error) {
	var out []hevyRoutineSummary
	page := 1
	for {
		var response struct {
			PageCount int                  `json:"page_count"`
			Routines  []hevyRoutineSummary `json:"routines"`
		}
		values := url.Values{}
		values.Set("page", strconv.Itoa(page))
		values.Set("pageSize", "10")
		if err := c.do(ctx, http.MethodGet, "/v1/routines?"+values.Encode(), nil, &response); err != nil {
			return nil, err
		}
		out = append(out, response.Routines...)
		if page >= response.PageCount || len(response.Routines) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (c *hevyClient) ListRoutineFolders(ctx context.Context) ([]hevyRoutineFolder, error) {
	var out []hevyRoutineFolder
	page := 1
	for {
		var response struct {
			PageCount      int                 `json:"page_count"`
			RoutineFolders []hevyRoutineFolder `json:"routine_folders"`
		}
		values := url.Values{}
		values.Set("page", strconv.Itoa(page))
		values.Set("pageSize", "10")
		if err := c.do(ctx, http.MethodGet, "/v1/routine_folders?"+values.Encode(), nil, &response); err != nil {
			return nil, err
		}
		out = append(out, response.RoutineFolders...)
		if page >= response.PageCount || len(response.RoutineFolders) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (c *hevyClient) CreateRoutine(ctx context.Context, request hevyRoutineRequest) (map[string]any, error) {
	var response map[string]any
	if err := c.do(ctx, http.MethodPost, "/v1/routines", request, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *hevyClient) UpdateRoutine(ctx context.Context, routineID string, request hevyRoutineRequest) (map[string]any, error) {
	var response map[string]any
	if err := c.do(ctx, http.MethodPut, "/v1/routines/"+url.PathEscape(routineID), request, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *hevyClient) CreateWorkout(ctx context.Context, request hevyWorkoutRequest) (map[string]any, error) {
	var response map[string]any
	if err := c.do(ctx, http.MethodPost, "/v1/workouts", request, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *hevyClient) CreateCustomExercise(ctx context.Context, request hevyCustomExerciseRequest) (string, error) {
	var response map[string]any
	if err := c.do(ctx, http.MethodPost, "/v1/exercise_templates", request, &response); err != nil {
		return "", err
	}
	id, ok := response["id"]
	if !ok {
		return "", fmt.Errorf("custom exercise response did not include id")
	}
	return fmt.Sprint(id), nil
}

func (c *hevyClient) do(ctx context.Context, method, path string, requestBody any, out any) error {
	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("api-key", c.apiKey)
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request %s %s failed with status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
