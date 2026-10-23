package openreplay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientNormalizesAPITrailingSlash(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()
	client, err := NewClient(server.URL+"/api/", "jwt")
	require.NoError(t, err)
	require.Equal(t, "/api/projects", client.URL("/projects").Path)
}

func TestNewClientNormalizesEndpoint(t *testing.T) {
	for _, endpoint := range []string{"https://example.test", "https://example.test/", "https://example.test/api", "http://example.test:8080/api/"} {
		t.Run(endpoint, func(t *testing.T) {
			client, err := NewClient(endpoint, "jwt")
			require.NoError(t, err)
			require.Equal(t, "/api/projects", client.URL("projects").Path)
			first := client.URL("/projects")
			first.Path = "/changed"
			require.Equal(t, "/api/projects", client.URL("/projects").Path)
		})
	}
}

func TestNewClientRejectsEndpointCredentials(t *testing.T) {
	_, err := NewClient("https://secret@example.test/api", "jwt")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
}

func TestNewClientRejectsInvalidEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"", "example.test/api", "//example.test/api", "ftp://example.test/api", "https:///api",
		"https://example.test/other", "https://example.test/api/projects", "https://example.test/api//",
		"https://example.test/api?secret=value", "https://example.test/api?", "https://example.test/api#secret",
		"https://example.test/api#", "https://secret:password@example.test/api", "https://example.test/%61pi",
		"https://secret%zz@example.test/api", "https://example.test:invalid/api",
	} {
		t.Run(endpoint, func(t *testing.T) {
			client, err := NewClient(endpoint, "jwt")
			require.Error(t, err)
			require.Nil(t, client)
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "password")
		})
	}
}

func TestNewClientRejectsMissingOrInvalidToken(t *testing.T) {
	for _, token := range []string{"", "  ", "secret\r\nAuthorization: injected"} {
		_, err := NewClient("https://example.test/api", token)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestClientDoSendsJSONAndDecodesData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/projects", r.URL.Path)
		assert.Equal(t, "Bearer private-jwt", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		var body map[string]string
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, map[string]string{"name": "Production", "platform": "web"}, body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"projectId":42,"name":"Production","extra":"ignored"},"errors":[]}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/", "private-jwt")
	require.NoError(t, err)
	var result struct {
		ID   int64  `json:"projectId"`
		Name string `json:"name"`
	}
	err = client.Do(context.Background(), http.MethodPost, "/projects", map[string]string{"name": "Production", "platform": "web"}, &result)
	require.NoError(t, err)
	require.Equal(t, int64(42), result.ID)
	require.Equal(t, "Production", result.Name)
}

func TestClientDoRejectsErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		message string
	}{
		{"errors envelope", 200, `{"errors":["unauthorized"]}`, "unauthorized"},
		{"errors with data", 200, `{"data":{"projectId":42},"errors":["failure"]}`, "failure"},
		{"unexpected errors shape", 200, `{"errors":{"message":"failure"}}`, "API returned errors"},
		{"validation detail", 400, `{"detail":"name already exists"}`, "name already exists"},
		{"unauthorized status", 401, `{"detail":"denied"}`, "owner/admin dashboard JWT"},
		{"forbidden status", 403, `{"detail":"denied"}`, "owner/admin dashboard JWT"},
		{"not found status", 404, `{"detail":"missing"}`, "404"},
		{"unparseable failure", 500, `<html>private-jwt</html>`, "500"},
		{"malformed JSON", 200, `{private-jwt`, "invalid JSON"},
		{"trailing JSON", 200, `{"data":{}} {"data":{}}`, "invalid JSON"},
		{"missing data", 200, `{}`, "missing data"},
		{"null data", 200, `{"data":null}`, "missing data"},
		{"wrong data type", 200, `{"data":"private-jwt"}`, "decode response data"},
		{"reflected token", 400, `{"detail":"rejected private-jwt"}`, "[REDACTED]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "private-jwt")
			require.NoError(t, err)
			var result map[string]any
			err = client.Do(context.Background(), http.MethodGet, "/projects/42", nil, &result)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.message)
			require.NotContains(t, err.Error(), "private-jwt")
		})
	}
}

func TestClientDoAllowsNoOutput(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{200, `{"data":{"state":"success"}}`},
		{204, ""},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, int64(0), r.ContentLength)
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		client, err := NewClient(server.URL, "jwt")
		require.NoError(t, err)
		err = client.Do(context.Background(), http.MethodDelete, "/projects/42", nil, nil)
		server.Close()
		require.NoError(t, err)
	}
}

func TestClientDoRejectsMissingDataWithoutOutput(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"data":null}`, `{"errors":[]}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "jwt")
			require.NoError(t, err)
			err = client.Do(context.Background(), http.MethodDelete, "/projects/42", nil, nil)
			require.ErrorContains(t, err, "missing data")
		})
	}
}

func TestClientDoHonorsContext(t *testing.T) {
	client, err := NewClient("https://example.test/api", "jwt")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.Do(ctx, http.MethodGet, "/projects/42", nil, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestClientDoDoesNotFollowRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects" {
			t.Error("client followed redirect")
		}
		http.Redirect(w, r, "/other", http.StatusFound)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "private-jwt")
	require.NoError(t, err)
	err = client.Do(context.Background(), http.MethodGet, "/projects", nil, nil)
	require.ErrorContains(t, err, "302")
}

func TestClientDoHidesEncodingAndTransportErrors(t *testing.T) {
	server := httptest.NewServer(nil)
	client, err := NewClient(server.URL, "private-jwt")
	require.NoError(t, err)
	server.Close()
	err = client.Do(context.Background(), http.MethodPost, "/projects", make(chan int), nil)
	require.ErrorContains(t, err, "encode request")
	err = client.Do(context.Background(), http.MethodGet, "/projects", nil, nil)
	require.ErrorContains(t, err, "request failed")
	require.NotContains(t, err.Error(), "private-jwt")
	require.False(t, strings.Contains(err.Error(), server.URL))
}
