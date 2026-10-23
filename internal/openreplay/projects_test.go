package openreplay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProjectSendsDashboardContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/projects", r.URL.Path)
		assert.Equal(t, "Bearer private-jwt", r.Header.Get("Authorization"))
		var body map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, map[string]any{"name": "Production", "platform": "web"}, body)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"projectId":42,"projectKey":"project-key","name":"Production","platform":"web","gdpr":{}},"errors":[]}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "private-jwt")
	require.NoError(t, err)

	project, err := client.CreateProject(context.Background(), ProjectInput{Name: "Production", Platform: "web"})

	require.NoError(t, err)
	require.Equal(t, Project{ID: 42, Key: "project-key", Name: "Production", Platform: "web"}, project)
}

func TestGetProjectSendsDashboardContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/projects/42", r.URL.Path)
		assert.Equal(t, "Bearer private-jwt", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Empty(t, body)
		_, _ = io.WriteString(w, `{"data":{"projectId":42,"projectKey":"project-key","name":"Production","platform":"web","tenantId":7}}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/", "private-jwt")
	require.NoError(t, err)

	project, err := client.GetProject(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, Project{ID: 42, Key: "project-key", Name: "Production", Platform: "web"}, project)
}

func TestUpdateProjectSendsDashboardContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/projects/42", r.URL.Path)
		assert.Equal(t, "Bearer private-jwt", r.Header.Get("Authorization"))
		var body map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, map[string]any{"name": "Renamed", "platform": "ios"}, body)
		_, _ = io.WriteString(w, `{"data":{"projectId":42,"projectKey":"project-key","name":"Renamed","platform":"ios"}}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "private-jwt")
	require.NoError(t, err)

	project, err := client.UpdateProject(context.Background(), 42, ProjectInput{Name: "Renamed", Platform: "ios"})

	require.NoError(t, err)
	require.Equal(t, Project{ID: 42, Key: "project-key", Name: "Renamed", Platform: "ios"}, project)
}

func TestDeleteProjectSendsDashboardContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"success envelope", http.StatusOK, `{"data":{"state":"success"}}`},
		{"no content", http.StatusNoContent, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, "/api/projects/42", r.URL.Path)
				assert.Equal(t, "Bearer private-jwt", r.Header.Get("Authorization"))
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Empty(t, body)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "private-jwt")
			require.NoError(t, err)

			require.NoError(t, client.DeleteProject(context.Background(), 42))
		})
	}
}

func TestGetProjectNotFound(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP 404", http.StatusNotFound, `{"detail":"missing"}`},
		{"HTTP 404 non JSON", http.StatusNotFound, "missing"},
		{"success status envelope", http.StatusOK, `{"errors":["project not found"]}`},
		{"failure status envelope", http.StatusInternalServerError, `{"errors":["project not found"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := projectResponseClient(t, tc.status, tc.body)

			_, err := client.GetProject(context.Background(), 42)

			require.ErrorIs(t, err, ErrNotFound)
			require.NotErrorIs(t, err, ErrUnauthorized)
		})
	}
}

func TestProjectUnauthorized(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP 401", http.StatusUnauthorized, "denied"},
		{"HTTP 403", http.StatusForbidden, `{"detail":"private-jwt rejected"}`},
		{"success status envelope", http.StatusOK, `{"errors":["unauthorized"]}`},
		{"failure status envelope", http.StatusInternalServerError, `{"errors":["unauthorized"]}`},
		{"error alongside data", http.StatusOK, `{"data":{"projectId":42},"errors":["private-jwt rejected","unauthorized"]}`},
		{"authorization status takes precedence", http.StatusForbidden, `{"errors":["project not found"]}`},
	} {
		for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodPut, http.MethodDelete} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				client := projectResponseClient(t, tc.status, tc.body)

				err := callProjectMethod(client, method)

				require.ErrorIs(t, err, ErrUnauthorized)
				require.NotErrorIs(t, err, ErrNotFound)
				require.NotContains(t, err.Error(), "private-jwt")
			})
		}
	}
}

func TestProjectRejectsErrorEnvelopes(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError} {
		for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodPut, http.MethodDelete} {
			t.Run(http.StatusText(status)+"/"+method, func(t *testing.T) {
				client := projectResponseClient(t, status, `{"data":{"projectId":42},"errors":["invalid project private-jwt"]}`)

				err := callProjectMethod(client, method)

				require.Error(t, err)
				require.ErrorContains(t, err, "invalid project")
				require.NotErrorIs(t, err, ErrNotFound)
				require.NotErrorIs(t, err, ErrUnauthorized)
				require.NotContains(t, err.Error(), "private-jwt")
			})
		}
	}
}

// projectResponseClient serves a fixed dashboard response to a real HTTP client.
func projectResponseClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, "private-jwt")
	require.NoError(t, err)
	return client
}

// callProjectMethod exercises each public CRUD method against a real server.
func callProjectMethod(client *Client, method string) error {
	ctx := context.Background()
	var err error
	switch method {
	case http.MethodPost:
		_, err = client.CreateProject(ctx, ProjectInput{Name: "Production", Platform: "web"})
	case http.MethodGet:
		_, err = client.GetProject(ctx, 42)
	case http.MethodPut:
		_, err = client.UpdateProject(ctx, 42, ProjectInput{Name: "Renamed", Platform: "web"})
	case http.MethodDelete:
		err = client.DeleteProject(ctx, 42)
	}
	return err
}
