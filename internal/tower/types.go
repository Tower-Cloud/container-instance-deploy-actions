package tower

// LoginRequest is the payload for POST /public?action=login
type LoginRequest struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	OrganizationID string `json:"organizationId"`
}

// LoginResponse — success response from the login endpoint.
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// APIError — error response format from IAM service.
type APIError struct {
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// ContainerResponse is the response from GET /service/container-instance/{name}
type ContainerResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Container struct {
			Name          string `json:"name"`
			Status        string `json:"status"`
			StatusReason  string `json:"statusReason,omitempty"`
			StatusMessage string `json:"statusMessage,omitempty"`
			Image         string `json:"image,omitempty"`
			ImageTag      string `json:"imageTag,omitempty"`
			Registry      string `json:"registry,omitempty"`
			RegistryType  string `json:"registryType,omitempty"`
			Replicas      int    `json:"replicas,omitempty"`
		} `json:"container"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

// RegistryCredentialsResponse is the response from GET /service/container-registry/credentials/{name}
type RegistryCredentialsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		RegistryURL string `json:"registryUrl"`
	} `json:"data"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// RegistryConfig holds all registry info needed for the update call.
type RegistryConfig struct {
	Type          string // "tower", "public", "private"
	RegistryURL   string // registry hostname
	ImageTag      string // repo/app:sha (without host, for public/private)
	FullImage     string // full URL (for tower towerImage)
	Username      string // for private registryCredentials
	Password      string // for private registryCredentials
	ContainerName string // for generating secret label
}

// UpdateContainerRequest is the payload for PUT /service/container-instance/{name}
type UpdateContainerRequest struct {
	ContainerSpec UpdateContainerSpec `json:"containerSpec"`
}

// UpdateContainerSpec holds the fields for updating a container.
type UpdateContainerSpec struct {
	RegistryType        string               `json:"registryType"`
	TowerImage          string               `json:"towerImage,omitempty"`
	Registry            string               `json:"registry,omitempty"`
	ImageTag            string               `json:"imageTag,omitempty"`
	RegistryCredentials *RegistryCredentials  `json:"registryCredentials,omitempty"`
}

// RegistryCredentials holds inline credentials for private registries.
type RegistryCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Label    string `json:"label"`
}

// UpdateContainerResponse is the response from the container update endpoint.
type UpdateContainerResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    struct {
		TaskID string `json:"taskId"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}
