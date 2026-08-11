package tower

// LoginRequest is the payload for POST /public?action=login (IAM service —
// unchanged by the container-instance v1 migration).
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

// APIError — legacy/IAM error response format ({error: {code, message}} OR
// just {error: "..."}). Kept for the login path which is not on v1 yet.
type APIError struct {
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// ---------------------------------------------------------------------------
// v1 envelopes — every container-instance v1 endpoint returns either
// `{"data": ...}` on success or `{"error": {"code", "message", "details",
// "requestId"}}` on failure. The IAM/registry endpoints still use the legacy
// shapes above.
// ---------------------------------------------------------------------------

// V1Error is the v1 error envelope. `Code` is a stable machine-readable
// identifier (e.g. REGISTRY_SECRET_LABEL_TAKEN); `Message` is user-safe text.
type V1Error struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
}

type V1ErrorEnvelope struct {
	Error *V1Error `json:"error,omitempty"`
}

// V1Image is the new structured image block on container detail responses
// (legacy returned `image`/`imageTag`/`registry` as separate top-level strings).
type V1Image struct {
	Repository   string `json:"repository"`
	Tag          string `json:"tag,omitempty"`
	Registry     string `json:"registry,omitempty"`
	RegistryType string `json:"registryType,omitempty"`
}

// V1Container is the public detail DTO returned by
// GET /service/container-instance/containers/{name}.
type V1Container struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	StatusReason  string  `json:"statusReason,omitempty"`
	StatusMessage string  `json:"statusMessage,omitempty"`
	Image         V1Image `json:"image"`
	URL           string  `json:"url,omitempty"`
	Replicas      int     `json:"replicas,omitempty"`
}

// V1ContainerResponse — top-level envelope around V1Container.
type V1ContainerResponse struct {
	Data struct {
		Container V1Container `json:"container"`
	} `json:"data"`
	Error *V1Error `json:"error,omitempty"`
}

// V1Credentials is the inline credential block on PATCH /image. `Label` is
// the saved-secret identifier under which v1 will store the credentials so
// future deploys can reference it via `credentialsLabel`.
type V1Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
	Label    string `json:"label"`
}

// V1PatchImageRequest is the body for
// PATCH /service/container-instance/containers/{name}/image.
//
// Exactly one of `Credentials` or `CredentialsLabel` may be set — both is a
// 400, neither means the image is treated as a public registry pull.
type V1PatchImageRequest struct {
	Image            string         `json:"image"`
	Credentials      *V1Credentials `json:"credentials,omitempty"`
	CredentialsLabel string         `json:"credentialsLabel,omitempty"`
}

// V1Operation is the operation block embedded in every async-mutation
// response. The `ID` field is what callers poll on at
// GET /service/container-instance/operations/{id}.
type V1Operation struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Status      string            `json:"status"`
	Container   string            `json:"container"`
	Message     string            `json:"message"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt,omitempty"`
	CompletedAt string            `json:"completedAt,omitempty"`
	Error       *V1OperationError `json:"error,omitempty"`
}

type V1OperationError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// V1OperationResponse is the 202-Accepted body returned by every mutating v1
// endpoint (PATCH /image, /resources, POST /start, etc).
type V1OperationResponse struct {
	Data struct {
		Operation V1Operation `json:"operation"`
	} `json:"data"`
	Error *V1Error `json:"error,omitempty"`
}

// RegistryCredentialsResponse — legacy shape from the container-registry
// service. The TCR credentials endpoint is not part of the v1 migration.
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

// RegistryConfig is internal state — what main.go assembles after resolving
// either Tower credentials or external creds, and what the API client
// translates into a V1PatchImageRequest.
type RegistryConfig struct {
	Type          string // "tower", "private"
	FullImage     string // full reference incl. host (host/repo/app:tag) — sent as v1 `image`
	Registry      string // private only registry host, used for secret-label scoping
	Username      string // private only
	Password      string // private only
	ContainerName string // used to derive the saved-secret label
}
