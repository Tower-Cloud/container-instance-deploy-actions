package tower

// LoginRequest is the payload for POST /public?action=login
type LoginRequest struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	OrganizationID string `json:"organizationId"`
}

// LoginResponse is the response from the login endpoint.
type LoginResponse struct {
	Success bool `json:"success"`
	Data    struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// ContainerResponse is the response from GET /service/container-instance/{name}
type ContainerResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Name         string `json:"name"`
		Status       string `json:"status"`
		StatusReason string `json:"statusReason,omitempty"`
		Image        string `json:"image,omitempty"`
		ImageTag     string `json:"imageTag,omitempty"`
		Registry     string `json:"registry,omitempty"`
		RegistryType string `json:"registryType,omitempty"`
		Replicas     int    `json:"replicas,omitempty"`
	} `json:"data"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// UpdateContainerRequest is the payload for PUT /service/container-instance/{name}
type UpdateContainerRequest struct {
	ContainerSpec ContainerSpec `json:"containerSpec"`
}

// ContainerSpec holds the image tag to update.
type ContainerSpec struct {
	ImageTag string `json:"imageTag"`
}

// UpdateContainerResponse is the response from the container update endpoint.
type UpdateContainerResponse struct {
	Success bool `json:"success"`
	Data    struct {
		TaskID string `json:"taskId"`
	} `json:"data"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}
