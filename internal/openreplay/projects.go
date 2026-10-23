package openreplay

import (
	"context"
	"fmt"
	"net/http"
)

// ProjectInput is the dashboard request body for creating or updating a project.
type ProjectInput struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// Project contains the dashboard fields managed by the provider.
type Project struct {
	ID       int64  `json:"projectId"`
	Key      string `json:"projectKey"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// CreateProject creates a project using an owner/admin dashboard JWT.
func (c *Client) CreateProject(ctx context.Context, input ProjectInput) (Project, error) {
	var project Project
	err := c.Do(ctx, http.MethodPost, "/projects", input, &project)
	return project, err
}

// GetProject reads a project by its numeric dashboard ID.
func (c *Client) GetProject(ctx context.Context, id int64) (Project, error) {
	var project Project
	err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d", id), nil, &project)
	return project, err
}

// UpdateProject updates a project's name and platform.
func (c *Client) UpdateProject(ctx context.Context, id int64, input ProjectInput) (Project, error) {
	var project Project
	err := c.Do(ctx, http.MethodPut, fmt.Sprintf("/projects/%d", id), input, &project)
	return project, err
}

// DeleteProject removes a project by its numeric dashboard ID.
func (c *Client) DeleteProject(ctx context.Context, id int64) error {
	return c.Do(ctx, http.MethodDelete, fmt.Sprintf("/projects/%d", id), nil, nil)
}
