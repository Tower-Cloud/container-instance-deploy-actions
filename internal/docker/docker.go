package docker

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Login authenticates with a Docker registry using --password-stdin.
// Returns a clear error distinguishing auth failure from registry not reachable.
func Login(registryURL, username, password string) error {
	cmd := exec.Command("docker", "login", registryURL, "-u", username, "--password-stdin")
	cmd.Stdin = strings.NewReader(password)

	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		lower := strings.ToLower(errMsg)

		switch {
		case strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401") || strings.Contains(lower, "authentication required"):
			return fmt.Errorf(
				"registry authentication failed for '%s'\n\n"+
					"Your registry_username or registry_password is incorrect.\n"+
					"Verify your credentials and try again",
				registryURL,
			)
		case strings.Contains(lower, "not found") || strings.Contains(lower, "404"):
			return fmt.Errorf(
				"registry '%s' not found\n\n"+
					"The registry_url does not point to a valid registry.\n"+
					"Check for typos — use the hostname only, without https://",
				registryURL,
			)
		case strings.Contains(lower, "no such host") || strings.Contains(lower, "could not resolve") || strings.Contains(lower, "connection refused"):
			return fmt.Errorf(
				"cannot reach registry '%s'\n\n"+
					"The registry URL is unreachable. Possible causes:\n"+
					"  - The URL is incorrect — check for typos, no https://\n"+
					"  - Network issue — the registry may be temporarily unavailable",
				registryURL,
			)
		default:
			return fmt.Errorf("docker login to '%s' failed: %s", registryURL, errMsg)
		}
	}
	return nil
}

// Build builds a Docker image for linux/amd64 (Tower Cloud cluster architecture).
func Build(imageURL, dockerfilePath, context string, buildArgs []string) error {
	args := []string{"build", "--platform", "linux/amd64", "-t", imageURL, "-f", dockerfilePath}

	for _, arg := range buildArgs {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			args = append(args, "--build-arg", arg)
		}
	}

	args = append(args, context)

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	return nil
}

// Push pushes a Docker image to the registry.
func Push(imageURL string) error {
	cmd := exec.Command("docker", "push", imageURL)

	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		lower := strings.ToLower(errMsg)

		switch {
		case strings.Contains(lower, "denied") || strings.Contains(lower, "unauthorized"):
			return fmt.Errorf(
				"permission denied pushing to registry\n\n"+
					"Your credentials may not have push access.\n"+
					"Verify that the provided credentials have write/push permissions",
			)
		default:
			return fmt.Errorf("docker push failed: %s", errMsg)
		}
	}
	return nil
}
