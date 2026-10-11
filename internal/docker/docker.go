package docker

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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

// ImageExists checks whether a tag already exists in the remote registry.
// Uses `docker buildx imagetools inspect` which is a cheap manifest HEAD.
// Returns (true, nil) if present, (false, nil) if not, or (false, err) on unexpected failure.
func ImageExists(imageURL string) (bool, error) {
	cmd := exec.Command("docker", "buildx", "imagetools", "inspect", imageURL)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		lower := strings.ToLower(stderr.String())
		if strings.Contains(lower, "not found") || strings.Contains(lower, "manifest unknown") || strings.Contains(lower, "no such manifest") {
			return false, nil
		}
		return false, nil
	}
	return true, nil
}

// BuildAndPush builds a linux/amd64 image with buildx and streams it to the
// registry in one step, using registry-based layer cache and zstd compression.
// Retries up to 3 times on transient network failures — our public ingress
// bandwidth is narrow and variable, so stalled streams are expected occasionally.
func BuildAndPush(imageURL, cacheRef, dockerfilePath, context string, buildArgs []string) error {
	args := []string{
		"buildx", "build",
		"--platform", "linux/amd64",
		"-t", imageURL,
		"-f", dockerfilePath,
		"--push",
		"--provenance=false",
		"--sbom=false",
		"--output", "type=image,compression=zstd,compression-level=3,force-compression=true",
	}

	if cacheRef != "" {
		args = append(args,
			"--cache-from", "type=registry,ref="+cacheRef,
			"--cache-to", "type=registry,ref="+cacheRef+",mode=max,compression=zstd",
		)
	}

	for _, arg := range buildArgs {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			args = append(args, "--build-arg", arg)
		}
	}

	args = append(args, context)

	// BUILDKIT_MAX_PARALLELISM=2 — on a ~0.5-1 MB/s public pipe, 2 concurrent
	// streams is the sweet spot. More streams fight for bandwidth and starve
	// each other (observed: one stream collapses to ~0.18 MB/s while others run).
	env := append(os.Environ(), "BUILDKIT_MAX_PARALLELISM=2")

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		cmd := exec.Command("docker", args...)
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if attempt > 1 {
			fmt.Printf("Retrying build+push (attempt %d/3)...\n", attempt)
		}

		if err := cmd.Run(); err != nil {
			lastErr = err
			lower := strings.ToLower(err.Error())
			// Retry on transient failures only.
			if strings.Contains(lower, "denied") || strings.Contains(lower, "unauthorized") {
				return fmt.Errorf(
					"permission denied pushing to registry\n\n"+
						"Your credentials may not have push access.\n"+
						"Verify that the provided credentials have write/push permissions",
				)
			}
			// Backoff: 5s, 15s
			if attempt < 3 {
				time.Sleep(time.Duration(attempt*10+5) * time.Second / 2)
				continue
			}
			return fmt.Errorf("docker buildx build --push failed after %d attempts: %w", attempt, err)
		}
		return nil
	}
	return lastErr
}
