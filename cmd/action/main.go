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

	mask(cfg.TowerPassword)
	mask(cfg.RegistryPassword)

	// Image URL: {registry_url}/{github_repo}/{container_name}:{github_sha}
	// e.g., my-registry.hyd.cr.tower.cloud/my-cool-app/my-container:abc123
	fullImageURL := fmt.Sprintf("%s/%s/%s:%s", cfg.RegistryURL, cfg.RepoName, cfg.ContainerName, cfg.GitHubSHA)

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

	status := containerResp.Data.Status
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
			cfg.ContainerName, containerResp.Data.StatusReason,
		)
	}

	fmt.Println("Preflight checks passed")
	endGroup()

	// ── Step 3: Docker Login ──
	group("Docker login")
	if err := docker.Login(cfg.RegistryURL, cfg.RegistryUsername, cfg.RegistryPassword); err != nil {
		return err
	}
	fmt.Printf("Logged into %s\n", cfg.RegistryURL)
	endGroup()

	// ── Step 4: Build Image (linux/amd64) ──
	group("Build container image")
	dockerfilePath := filepath.Join(cfg.AppSourcePath, cfg.DockerfilePath)
	buildArgs := parseBuildArgs(cfg.BuildArguments)

	fmt.Printf("Building image: %s (platform: linux/amd64)\n", fullImageURL)
	if err := docker.Build(fullImageURL, dockerfilePath, cfg.AppSourcePath, buildArgs); err != nil {
		return err
	}
	fmt.Println("Image built successfully")
	endGroup()

	// ── Step 5: Push Image ──
	group("Push container image")
	fmt.Printf("Pushing image: %s\n", fullImageURL)
	if err := docker.Push(fullImageURL); err != nil {
		return err
	}
	fmt.Println("Image pushed successfully")
	endGroup()

	// ── Step 6: Re-authenticate + Update Container Instance ──
	// Token may have expired during docker build/push — get a fresh one.
	group("Update container instance")
	fmt.Println("Re-authenticating before deploy...")
	token, err = client.Login(cfg.TowerUser, cfg.TowerPassword, cfg.OrganizationID)
	if err != nil {
		return fmt.Errorf("re-authentication failed: %w", err)
	}
	mask(token)

	fmt.Printf("Updating container instance '%s' with image: %s\n", cfg.ContainerName, fullImageURL)
	taskID, err := client.UpdateContainer(token, cfg.OrganizationID, cfg.ContainerName, fullImageURL)
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
	RegistryURL      string
	RegistryUsername  string
	RegistryPassword string
	BuildArguments   string
	RepoName         string // extracted from GITHUB_REPOSITORY (owner/repo → repo)
	GitHubSHA        string
}

func readConfig() config {
	// GITHUB_REPOSITORY is "owner/repo-name" — extract just the repo name.
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
	if cfg.RegistryURL == "" {
		missing = append(missing, "registry_url")
	}
	if cfg.RegistryUsername == "" {
		missing = append(missing, "registry_username")
	}
	if cfg.RegistryPassword == "" {
		missing = append(missing, "registry_password")
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"missing required inputs: %s\n\n"+
				"Prerequisites before using this action:\n"+
				"  1. Create a Tower Cloud account at https://console.tower.cloud\n"+
				"  2. Create a Container Registry (TCR) and note the registry URL\n"+
				"  3. Create a Container Instance (TCI) using an image from that registry\n"+
				"  4. Add credentials as GitHub secrets\n"+
				"  5. Provide container_name and registry_url in the workflow inputs",
			strings.Join(missing, ", "),
		)
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
