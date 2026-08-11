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
//
// Container endpoints under `/service/container-instance/...` are served by
// the container-provisioning-service v1 API (mutations are async with 202 +
// operation envelope; org is derived from the JWT, no X-Organization-ID
// header). Login and registry-credentials endpoints are still on legacy
// paths.
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

// Login authenticates with Tower Cloud and returns an access token. IAM
// service — unchanged by the container-instance v1 migration.
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
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Error.Message != "" {
			return "", fmt.Errorf("login failed (%s): %s", apiErr.Error.Code, apiErr.Error.Message)
		}
		return "", fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return "", fmt.Errorf("failed to parse login response: %w", err)
	}

	if loginResp.AccessToken == "" {
		return "", fmt.Errorf("login succeeded but no access token returned (response: %s)", string(respBody))
	}

	return loginResp.AccessToken, nil
}

// containerPath returns the v1 public path for a single container resource.
// The api-mgmt gateway maps `/service/container-instance/containers/...` to
// the backend `/api/v1/containers/...` route, so callers never see `/v1` in
// the URL.
func (c *Client) containerPath(name string) string {
	return fmt.Sprintf("%s/service/container-instance/containers/%s", c.apiURL, name)
}

// GetContainer fetches the v1 detail view for an existing container. Returns
// a friendly error if the container doesn't exist (the action only updates,
// never creates).
//
// Note: v1 derives organization context from the JWT azp claim — no
// X-Organization-ID header is sent (or accepted). The orgID parameter is
// retained for symmetry with the legacy signature but unused.
func (c *Client) GetContainer(token, _orgID, containerName string) (*V1Container, error) {
	req, err := http.NewRequest("GET", c.containerPath(containerName), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get container request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

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
				"  1. Go to https://portal.dev.tower.cloud\n"+
				"  2. Navigate to Container Instances > Create Instance\n"+
				"  3. Use the instance name as the 'container_name' input in your workflow",
			containerName,
		)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseV1Error(resp.StatusCode, respBody, "get container instance")
	}

	var containerResp V1ContainerResponse
	if err := json.Unmarshal(respBody, &containerResp); err != nil {
		return nil, fmt.Errorf("failed to parse get container response: %w (body: %s)", err, string(respBody))
	}

	return &containerResp.Data.Container, nil
}

// GetRegistryCredentials fetches Tower registry credentials from the
// container-registry service. Legacy envelope — not part of the v1
// container-instance migration.
func (c *Client) GetRegistryCredentials(token, _orgID, tcrName string) (registryURL, username, password string, err error) {
	url := fmt.Sprintf("%s/service/container-registry/credentials/%s", c.apiURL, tcrName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create registry credentials request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("registry credentials request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read registry credentials response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", "", "", fmt.Errorf(
			"tower registry '%s' not found\n\n"+
				"Please create the container registry first via the Tower Cloud portal:\n"+
				"  1. Go to https://portal.dev.tower.cloud\n"+
				"  2. Navigate to Container Registries > Create Registry\n"+
				"  3. Use the registry name as the 'tcr_name' input",
			tcrName,
		)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("failed to get registry credentials (status %d): %s", resp.StatusCode, string(respBody))
	}

	var credsResp RegistryCredentialsResponse
	if err := json.Unmarshal(respBody, &credsResp); err != nil {
		return "", "", "", fmt.Errorf("failed to parse registry credentials response: %w", err)
	}

	if !credsResp.Success {
		return "", "", "", fmt.Errorf("failed to get registry credentials: %s - %s", credsResp.Error, credsResp.Message)
	}

	return credsResp.Data.RegistryURL, credsResp.Data.Username, credsResp.Data.Password, nil
}

// secretLabelFor returns the saved-credential label this action uses across
// deploys of a given container. Stable per container so the backend can upsert
// the same Kubernetes pull secret on each deploy instead of creating a new one.
func secretLabelFor(containerName string) string {
	return containerName + "-registry-secret"
}

// PatchContainerImage updates the image on an existing container via the v1
// atomic PATCH endpoint. Returns the operation id from the 202 envelope.
//
// Registry-handling matrix:
//   - Tower (regCfg.Type == "tower"): body is just `{"image": "<full URL>"}`.
//     v1 detects the Tower glob (*.cr.tower.cloud) and routes through the
//     org-level pull secret. No credentials are sent.
//   - Private external: first attempt sends inline `credentials` with a
//     stable per-container label. The v1 backend owns idempotency by upserting
//     that saved secret on each deploy. The action deliberately does not fall
//     back to `credentialsLabel`, because that can reuse stale or truncated
//     credentials even when the workflow supplied fresh registry secrets.
func (c *Client) PatchContainerImage(token, _orgID, containerName string, regCfg RegistryConfig) (string, error) {
	url := c.containerPath(containerName) + "/image"

	switch regCfg.Type {
	case "tower":
		return c.patchImage(url, token, V1PatchImageRequest{Image: regCfg.FullImage})
	case "private":
		label := secretLabelFor(regCfg.ContainerName)
		return c.patchImage(url, token, V1PatchImageRequest{
			Image: regCfg.FullImage,
			Credentials: &V1Credentials{
				Username: regCfg.Username,
				Password: regCfg.Password,
				Label:    label,
			},
		})
	default:
		return "", fmt.Errorf("unsupported registry type %q (expected \"tower\" or \"private\")", regCfg.Type)
	}
}

// patchImage performs a single PATCH attempt and decodes the v1 envelope.
// 200 and 202 are both treated as success — the controller normally returns
// 202 (async) but we accept 200 defensively.
func (c *Client) patchImage(url, token string, payload V1PatchImageRequest) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal patch image request: %w", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create patch image request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("patch image request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read patch image response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", parseV1Error(resp.StatusCode, respBody, "update container image")
	}

	var opResp V1OperationResponse
	if err := json.Unmarshal(respBody, &opResp); err != nil {
		return "", fmt.Errorf("failed to parse patch image response: %w (body: %s)", err, string(respBody))
	}
	if opResp.Data.Operation.ID == "" {
		return "", fmt.Errorf("patch image succeeded but no operation id was returned (body: %s)", string(respBody))
	}
	return opResp.Data.Operation.ID, nil
}

func (c *Client) operationPath(operationID string) string {
	return fmt.Sprintf("%s/service/container-instance/operations/%s", c.apiURL, operationID)
}

func (c *Client) GetOperation(token, _orgID, operationID string) (*V1Operation, error) {
	req, err := http.NewRequest("GET", c.operationPath(operationID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get operation request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get operation request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read get operation response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseV1Error(resp.StatusCode, respBody, "get operation status")
	}

	var opResp V1OperationResponse
	if err := json.Unmarshal(respBody, &opResp); err != nil {
		return nil, fmt.Errorf("failed to parse get operation response: %w (body: %s)", err, string(respBody))
	}
	if opResp.Data.Operation.ID == "" {
		return nil, fmt.Errorf("get operation succeeded but no operation was returned (body: %s)", string(respBody))
	}
	return &opResp.Data.Operation, nil
}

func (c *Client) WaitForOperation(token, orgID, operationID string, timeout time.Duration) error {
	if operationID == "" {
		return fmt.Errorf("operation id is required")
	}

	deadline := time.Now().Add(timeout)
	lastStatus := ""

	for {
		op, err := c.GetOperation(token, orgID, operationID)
		if err != nil {
			return err
		}

		if op.Status != lastStatus {
			if op.Message != "" {
				fmt.Printf("Operation status: %s - %s\n", op.Status, op.Message)
			} else {
				fmt.Printf("Operation status: %s\n", op.Status)
			}
			lastStatus = op.Status
		}

		switch op.Status {
		case "succeeded":
			return nil
		case "failed":
			return operationFailureError(op)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for container update operation %s (last status: %s)", operationID, op.Status)
		}
		time.Sleep(5 * time.Second)
	}
}

func operationFailureError(op *V1Operation) error {
	if op.Error != nil {
		switch {
		case op.Error.Code != "" && op.Error.Message != "":
			return fmt.Errorf("container update operation failed (%s): %s", op.Error.Code, op.Error.Message)
		case op.Error.Message != "":
			return fmt.Errorf("container update operation failed: %s", op.Error.Message)
		case op.Error.Code != "":
			return fmt.Errorf("container update operation failed (%s)", op.Error.Code)
		}
	}
	if op.Message != "" {
		return fmt.Errorf("container update operation failed: %s", op.Message)
	}
	return fmt.Errorf("container update operation failed")
}

// v1HTTPError carries the v1 error code so callers can branch on it.
type v1HTTPError struct {
	Status  int
	Code    string
	Message string
}

func (e *v1HTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s (%s, status %d)", e.Message, e.Code, e.Status)
	}
	return fmt.Sprintf("%s (status %d)", e.Message, e.Status)
}

// parseV1Error decodes a v1 error envelope into a friendly error. Falls back
// to the raw body when the response is not in v1 shape (e.g. gateway 502).
func parseV1Error(status int, body []byte, action string) error {
	var env V1ErrorEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error != nil {
		return &v1HTTPError{Status: status, Code: env.Error.Code, Message: fmt.Sprintf("failed to %s: %s", action, env.Error.Message)}
	}
	return fmt.Errorf("failed to %s (status %d): %s", action, status, string(body))
}
