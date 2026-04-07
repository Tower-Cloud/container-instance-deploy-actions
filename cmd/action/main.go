package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tower-cloud/container-instance-deploy-actions/internal/docker"
	"github.com/tower-cloud/container-instance-deploy-actions/internal/tower"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "::error::%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := readConfig()

	if err := validateConfig(cfg); err != nil {
		return err
	}

	// Mask secrets immediately.
	mask(cfg.TowerPassword)
	mask(cfg.RegistryPassword)

	// Determine registry path: tower or external (public/private).
	isTower := cfg.TCRName != ""

	// Short SHA for image tag.
	shortSHA := cfg.GitHubSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	repoName := strings.ToLower(cfg.RepoName)
	containerName := strings.ToLower(cfg.ContainerName)

	// ── Step 1: Login to Tower Cloud ──
	group("Login to Tower Cloud")
	apiURL := os.Getenv("TOWER_API_URL")
	if apiURL == "" {
		return fmt.Errorf("TOWER_API_URL is not configured — contact Tower Cloud support")
	}

	client := tower.NewClient(apiURL)

	token, err := client.Login(cfg.TowerUser, cfg.TowerPassword, cfg.OrganizationID)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	mask(token)
	fmt.Println("Successfully authenticated with Tower Cloud")
	endGroup()

	// ── Step 2: Preflight — verify container instance exists and is updatable ──
	group("Preflight checks")

	fmt.Printf("Checking container instance '%s'...\n", cfg.ContainerName)
	containerResp, err := client.GetContainer(token, cfg.OrganizationID, cfg.ContainerName)
	if err != nil {
		return err
	}

	status := containerResp.Data.Container.Status
	fmt.Printf("Container instance '%s' found (status: %s)\n", cfg.ContainerName, status)

	switch status {
	case "provisioning", "pending":
		return fmt.Errorf(
			"container instance '%s' is currently '%s' — cannot update while a previous operation is in progress\n\n"+
				"Wait for the current operation to complete before deploying again",
			cfg.ContainerName, status,
		)
	case "failed":
		return fmt.Errorf(
			"container instance '%s' is in 'failed' state (reason: %s)\n\n"+
				"Resolve the issue in the Tower Cloud portal before attempting to deploy.\n"+
				"You may need to delete and recreate the container instance",
			cfg.ContainerName, containerResp.Data.Container.StatusReason,
		)
	}

	fmt.Println("Preflight checks passed")
	endGroup()

	// ── Step 3: Resolve registry credentials ──
	var registryURL, regUser, regPass string
	var regCfg tower.RegistryConfig

	if isTower {
		// Tower registry — fetch credentials from API.
		group("Fetch Tower registry credentials")
		fmt.Printf("Fetching credentials for Tower registry '%s'...\n", cfg.TCRName)

		registryURL, regUser, regPass, err = client.GetRegistryCredentials(token, cfg.OrganizationID, cfg.TCRName)
		if err != nil {
			return err
		}
		mask(regPass)
		fmt.Printf("Registry: %s\n", registryURL)
		endGroup()

		// Image: {registry_url}/{repo}/{container}:{sha}
		fullImageURL := fmt.Sprintf("%s/%s/%s:%s", registryURL, repoName, containerName, shortSHA)

		regCfg = tower.RegistryConfig{
			Type:          "tower",
			RegistryURL:   registryURL,
			FullImage:     fullImageURL,
			ContainerName: cfg.ContainerName,
		}
	} else {
		// External registry.
		registryURL = cfg.RegistryURL
		regUser = cfg.RegistryUsername
		regPass = cfg.RegistryPassword

		// Auto-detect Tower registry URL — use IAM credentials if no registry creds provided.
		isTowerURL := strings.Contains(registryURL, "tower.cloud")
		if isTowerURL && regUser == "" && regPass == "" {
			fmt.Println("Tower registry URL detected — using IAM credentials for registry authentication.")
			regUser = cfg.TowerUser
			regPass = cfg.TowerPassword
			cfg.RegistryType = "tower"
		}

		// imageTag for API: repo/container:sha (without registry host)
		imageTag := fmt.Sprintf("%s/%s:%s", repoName, containerName, shortSHA)

		if cfg.RegistryType == "tower" {
			// Tower URL without tcr_name — treat as tower type.
			fullImg := fmt.Sprintf("%s/%s/%s:%s", registryURL, repoName, containerName, shortSHA)
			regCfg = tower.RegistryConfig{
				Type:          "tower",
				RegistryURL:   registryURL,
				FullImage:     fullImg,
				ContainerName: cfg.ContainerName,
			}
		} else {
			regCfg = tower.RegistryConfig{
				Type:          cfg.RegistryType,
				RegistryURL:   registryURL,
				ImageTag:      imageTag,
				Username:      regUser,
				Password:      regPass,
				ContainerName: cfg.ContainerName,
			}
		}
	}

	// Full docker tag: {registry_url}/{repo}/{container}:{sha}
	fullImageURL := fmt.Sprintf("%s/%s/%s:%s", registryURL, repoName, containerName, shortSHA)
	if isTower {
		fullImageURL = regCfg.FullImage
	}

	fmt.Printf("Image: %s\n", fullImageURL)

	// ── Step 4: Docker Login ──
	if regUser != "" && regPass != "" {
		group("Docker login")
		if err := docker.Login(registryURL, regUser, regPass); err != nil {
			return err
		}
		fmt.Printf("Logged into %s\n", registryURL)
		endGroup()
	} else {
		fmt.Printf("Skipping docker login — public registry '%s' (no credentials needed)\n", registryURL)
	}

	// ── Step 5: Build Image (linux/amd64) ──
	group("Build container image")
	dockerfilePath := filepath.Join(cfg.AppSourcePath, cfg.DockerfilePath)
	buildArgs := parseBuildArgs(cfg.BuildArguments)

	fmt.Printf("Building image: %s (platform: linux/amd64)\n", fullImageURL)
	if err := docker.Build(fullImageURL, dockerfilePath, cfg.AppSourcePath, buildArgs); err != nil {
		return err
	}
	fmt.Println("Image built successfully")
	endGroup()

	// ── Step 6: Push Image ──
	group("Push container image")
	fmt.Printf("Pushing image: %s\n", fullImageURL)
	if err := docker.Push(fullImageURL); err != nil {
		return err
	}
	fmt.Println("Image pushed successfully")
	endGroup()

	// ── Step 7: Re-authenticate + Update Container Instance ──
	group("Update container instance")
	fmt.Println("Re-authenticating before deploy...")
	token, err = client.Login(cfg.TowerUser, cfg.TowerPassword, cfg.OrganizationID)
	if err != nil {
		return fmt.Errorf("re-authentication failed: %w", err)
	}
	mask(token)

	fmt.Printf("Updating container instance '%s' (registry type: %s)\n", cfg.ContainerName, regCfg.Type)
	taskID, err := client.UpdateContainer(token, cfg.OrganizationID, cfg.ContainerName, regCfg)
	if err != nil {
		return fmt.Errorf("failed to update container instance: %w", err)
	}
	fmt.Printf("Container update accepted (taskId: %s)\n", taskID)
	endGroup()

	setOutput("taskId", taskID)
	setOutput("imageUrl", fullImageURL)

	fmt.Println("")
	fmt.Println("Deployment successful!")
	fmt.Printf("  Image:  %s\n", fullImageURL)
	fmt.Printf("  TaskID: %s\n", taskID)

	return nil
}

type config struct {
	AppSourcePath    string
	DockerfilePath   string
	TowerUser        string
	TowerPassword    string
	OrganizationID   string
	ContainerName    string
	TCRName          string // Tower registry name (if tower)
	RegistryType     string // "public" or "private" (if external)
	RegistryURL      string // External registry URL
	RegistryUsername  string // External registry username
	RegistryPassword string // External registry password
	BuildArguments   string
	RepoName         string
	GitHubSHA        string
}

func readConfig() config {
	repoName := ""
	if fullRepo := os.Getenv("GITHUB_REPOSITORY"); fullRepo != "" {
		parts := strings.SplitN(fullRepo, "/", 2)
		if len(parts) == 2 {
			repoName = parts[1]
		}
	}

	return config{
		AppSourcePath:    envOrDefault("INPUT_APP_SOURCE_PATH", "."),
		DockerfilePath:   envOrDefault("INPUT_DOCKERFILE_PATH", "Dockerfile"),
		TowerUser:        os.Getenv("INPUT_TOWER_USER"),
		TowerPassword:    os.Getenv("INPUT_TOWER_PASSWORD"),
		OrganizationID:   os.Getenv("INPUT_ORGANIZATION_ID"),
		ContainerName:    os.Getenv("INPUT_CONTAINER_NAME"),
		TCRName:          os.Getenv("INPUT_TCR_NAME"),
		RegistryType:     envOrDefault("INPUT_REGISTRY_TYPE", "private"),
		RegistryURL:      os.Getenv("INPUT_REGISTRY_URL"),
		RegistryUsername:  os.Getenv("INPUT_REGISTRY_USERNAME"),
		RegistryPassword: os.Getenv("INPUT_REGISTRY_PASSWORD"),
		BuildArguments:   os.Getenv("INPUT_BUILD_ARGUMENTS"),
		RepoName:         repoName,
		GitHubSHA:        os.Getenv("GITHUB_SHA"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseBuildArgs(raw string) []string {
	if raw == "" {
		return nil
	}
	var args []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			args = append(args, line)
		}
	}
	return args
}

func validateConfig(cfg config) error {
	var missing []string

	if cfg.TowerUser == "" {
		missing = append(missing, "tower_user")
	}
	if cfg.TowerPassword == "" {
		missing = append(missing, "tower_password")
	}
	if cfg.OrganizationID == "" {
		missing = append(missing, "organization_id")
	}
	if cfg.ContainerName == "" {
		missing = append(missing, "container_name")
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"missing required inputs: %s\n\n"+
				"Prerequisites before using this action:\n"+
				"  1. Create a Tower Cloud account at https://portal.dev.tower.cloud\n"+
				"  2. Create a Container Registry and a Container Instance\n"+
				"  3. Add credentials as GitHub secrets\n"+
				"  4. Provide container_name in the workflow inputs",
			strings.Join(missing, ", "),
		)
	}

	isTower := cfg.TCRName != ""

	if !isTower {
		// External registry — need URL at minimum.
		if cfg.RegistryURL == "" {
			return fmt.Errorf(
				"missing required input: registry_url\n\n"+
					"For external registries (Docker Hub, GHCR, etc.), provide:\n"+
					"  - registry_url (always required)\n"+
					"  - registry_username, registry_password (required for private registries)\n\n"+
					"For Tower registries, provide:\n"+
					"  - tcr_name (credentials are fetched automatically)",
			)
		}

		// Private registry requires credentials.
		if cfg.RegistryType == "private" && (cfg.RegistryUsername == "" || cfg.RegistryPassword == "") {
			// Check if it's a Tower URL — IAM creds will be used automatically.
			if !strings.Contains(cfg.RegistryURL, "tower.cloud") {
				return fmt.Errorf(
					"missing registry credentials for private registry\n\n"+
						"For private registries, provide:\n"+
						"  - registry_username, registry_password\n\n"+
						"For public registries, set registry_type to 'public'\n"+
						"For Tower registries, provide tcr_name or use a .tower.cloud registry URL",
				)
			}
		}

		// Validate registry_type.
		if cfg.RegistryType != "public" && cfg.RegistryType != "private" {
			return fmt.Errorf(
				"invalid registry_type '%s' — must be 'public' or 'private'",
				cfg.RegistryType,
			)
		}
	}

	if cfg.RepoName == "" {
		return fmt.Errorf("GITHUB_REPOSITORY is not set — this action must run within a GitHub Actions workflow")
	}

	if cfg.GitHubSHA == "" {
		return fmt.Errorf("GITHUB_SHA is not set — this action must run within a GitHub Actions workflow")
	}

	return nil
}

// GitHub Actions helpers.

func mask(value string) {
	if value != "" {
		fmt.Printf("::add-mask::%s\n", value)
	}
}

func group(name string) {
	fmt.Printf("::group::%s\n", name)
}

func endGroup() {
	fmt.Println("::endgroup::")
}

func setOutput(name, value string) {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		fmt.Printf("::set-output name=%s::%s\n", name, value)
		return
	}
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "::warning::Failed to write output %s: %v\n", name, err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s=%s\n", name, value)
}
