package tower

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles communication with the Tower Cloud API gateway.
type Client struct {
	apiURL     string
	httpClient *http.Client
}

// NewClient creates a new Tower API client.
func NewClient(apiURL string) *Client {
	return &Client{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Login authenticates with Tower Cloud and returns an access token.
func (c *Client) Login(username, password, orgID string) (string, error) {
	payload := LoginRequest{
		Username:       username,
		Password:       password,
		OrganizationID: orgID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal login request: %w", err)
	}

	req, err := http.NewRequest("POST", c.apiURL+"/public?action=login", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return "", fmt.Errorf("failed to parse login response: %w", err)
	}

	if !loginResp.Success {
		return "", fmt.Errorf("login failed: %s - %s", loginResp.Error, loginResp.Message)
	}

	if loginResp.Data.AccessToken == "" {
		return "", fmt.Errorf("login succeeded but no access token returned")
	}

	return loginResp.Data.AccessToken, nil
}

// GetContainer checks if a container instance exists and returns its details.
func (c *Client) GetContainer(token, orgID, tciName string) (*ContainerResponse, error) {
	url := fmt.Sprintf("%s/service/container-instance/%s", c.apiURL, tciName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get container request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Organization-ID", orgID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get container request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read get container response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf(
			"container instance '%s' not found\n\n"+
				"This action only updates existing container instances — it does not create new ones.\n"+
				"Please create the container instance first via the Tower Cloud portal:\n"+
				"  1. Go to https://console.tower.cloud\n"+
				"  2. Navigate to Container Instances > Create Instance\n"+
				"  3. Use the instance name as the 'container_name' input in your workflow",
			tciName,
		)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get container instance (status %d): %s", resp.StatusCode, string(respBody))
	}

	var containerResp ContainerResponse
	if err := json.Unmarshal(respBody, &containerResp); err != nil {
		return nil, fmt.Errorf("failed to parse get container response: %w", err)
	}

	if !containerResp.Success {
		return nil, fmt.Errorf("failed to get container instance: %s - %s", containerResp.Error, containerResp.Message)
	}

	return &containerResp, nil
}

// UpdateContainer updates an existing container instance with a new image tag.
func (c *Client) UpdateContainer(token, orgID, tciName, imageTag string) (string, error) {
	url := fmt.Sprintf("%s/service/container-instance/%s", c.apiURL, tciName)

	payload := UpdateContainerRequest{
		ContainerSpec: ContainerSpec{
			ImageTag: imageTag,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal update request: %w", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Organization-ID", orgID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("update container request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read update response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("update container failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var updateResp UpdateContainerResponse
	if err := json.Unmarshal(respBody, &updateResp); err != nil {
		return "", fmt.Errorf("failed to parse update response: %w", err)
	}

	if !updateResp.Success {
		return "", fmt.Errorf("update container failed: %s - %s", updateResp.Error, updateResp.Message)
	}

	return updateResp.Data.TaskID, nil
}
