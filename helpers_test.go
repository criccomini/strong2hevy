package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseFolderArgUsesExistingNamedFolder(t *testing.T) {
	t.Parallel()

	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/routine_folders":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"page_count":1,"routine_folders":[{"id":7,"title":"Existing Folder"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/routine_folders":
			createCalls++
			http.Error(w, "unexpected create", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestHevyClient(server)
	id, err := parseFolderArg(context.Background(), client, "existing folder")
	if err != nil {
		t.Fatalf("parseFolderArg returned error: %v", err)
	}
	if id == nil || *id != 7 {
		t.Fatalf("parseFolderArg returned %v, want id 7", id)
	}
	if createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0", createCalls)
	}
}

func TestParseFolderArgCreatesMissingNamedFolder(t *testing.T) {
	t.Parallel()

	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/routine_folders":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"page_count":1,"routine_folders":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/routine_folders":
			createCalls++
			var request hevyRoutineFolderRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request body: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if request.RoutineFolder.Title != "New Folder" {
				t.Errorf("request title = %q, want %q", request.RoutineFolder.Title, "New Folder")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":42,"title":"New Folder"}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestHevyClient(server)
	id, err := parseFolderArg(context.Background(), client, "  New Folder  ")
	if err != nil {
		t.Fatalf("parseFolderArg returned error: %v", err)
	}
	if id == nil || *id != 42 {
		t.Fatalf("parseFolderArg returned %v, want id 42", id)
	}
	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", createCalls)
	}
}

func TestParseFolderArgRecoversFolderIDWhenCreateResponseOmitsID(t *testing.T) {
	t.Parallel()

	listCalls := 0
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/routine_folders":
			listCalls++
			w.Header().Set("Content-Type", "application/json")
			if listCalls == 1 {
				_, _ = io.WriteString(w, `{"page_count":1,"routine_folders":[]}`)
				return
			}
			_, _ = io.WriteString(w, `{"page_count":1,"routine_folders":[{"id":84,"title":"Recovered Folder"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/routine_folders":
			createCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"routine_folder":{"title":"Recovered Folder"}}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestHevyClient(server)
	id, err := parseFolderArg(context.Background(), client, "Recovered Folder")
	if err != nil {
		t.Fatalf("parseFolderArg returned error: %v", err)
	}
	if id == nil || *id != 84 {
		t.Fatalf("parseFolderArg returned %v, want id 84", id)
	}
	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", createCalls)
	}
	if listCalls != 2 {
		t.Fatalf("listCalls = %d, want 2", listCalls)
	}
}

func newTestHevyClient(server *httptest.Server) *hevyClient {
	client := newHevyClient("test-api-key")
	client.baseURL = server.URL
	client.httpClient = server.Client()
	return client
}
