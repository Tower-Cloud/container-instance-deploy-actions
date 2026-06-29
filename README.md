# Tower Cloud Container Deploy Action

Build, push, and deploy a container image to Tower Cloud — in a single step.

Works with **Tower registries**, **Docker Hub**, **GHCR**, **GCR**, **ECR**, or any Docker-compatible registry.

## Usage

### Tower Registry

Just provide `tcr_name` — credentials are fetched automatically.

```yaml
name: Deploy to Tower Cloud
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build and deploy
        uses: tower-cloud/container-instance-deploy-actions@main
        with:
          tower_user: ${{ secrets.TOWER_USER }}
          tower_password: ${{ secrets.TOWER_PASSWORD }}
          organization_id: ${{ secrets.TOWER_ORG_ID }}
          container_name: my-app
          tcr_name: ${{ secrets.TCR_NAME }}
```

### External Registry (Docker Hub, GHCR, ACR, GCR, ECR, etc.)

Provide registry URL and credentials. Works for both public and private registries — credentials are always needed for push.

```yaml
      - name: Build and deploy
        uses: tower-cloud/container-instance-deploy-actions@main
        with:
          tower_user: ${{ secrets.TOWER_USER }}
          tower_password: ${{ secrets.TOWER_PASSWORD }}
          organization_id: ${{ secrets.TOWER_ORG_ID }}
          container_name: my-app
          registry_url: ${{ secrets.REGISTRY_URL }}
          registry_username: ${{ secrets.REGISTRY_USERNAME }}
          registry_password: ${{ secrets.REGISTRY_PASSWORD }}
```

### Custom Dockerfile Path

If your Dockerfile is not in the project root:

```yaml
      - name: Build and deploy
        uses: tower-cloud/container-instance-deploy-actions@main
        with:
          tower_user: ${{ secrets.TOWER_USER }}
          tower_password: ${{ secrets.TOWER_PASSWORD }}
          organization_id: ${{ secrets.TOWER_ORG_ID }}
          container_name: my-app
          tcr_name: ${{ secrets.TCR_NAME }}
          dockerfilePath: docker/Dockerfile.prod
```

### Build Arguments

Pass Docker build arguments as multiline key=value pairs:

```yaml
      - name: Build and deploy
        uses: tower-cloud/container-instance-deploy-actions@main
        with:
          tower_user: ${{ secrets.TOWER_USER }}
          tower_password: ${{ secrets.TOWER_PASSWORD }}
          organization_id: ${{ secrets.TOWER_ORG_ID }}
          container_name: my-app
          tcr_name: ${{ secrets.TCR_NAME }}
          buildArguments: |
            NODE_ENV=production
            API_URL=https://api.example.com
            BUILD_VERSION=${{ github.sha }}
```

### Using Outputs

Access the deployment task ID and image URL in subsequent steps:

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build and deploy
        id: deploy
        uses: tower-cloud/container-instance-deploy-actions@main
        with:
          tower_user: ${{ secrets.TOWER_USER }}
          tower_password: ${{ secrets.TOWER_PASSWORD }}
          organization_id: ${{ secrets.TOWER_ORG_ID }}
          container_name: my-app
          tcr_name: ${{ secrets.TCR_NAME }}

      - name: Print deployment info
        run: |
          echo "Task ID: ${{ steps.deploy.outputs.taskId }}"
          echo "Image:   ${{ steps.deploy.outputs.imageUrl }}"

      - name: Notify on success
        if: success()
        run: echo "Deployed ${{ steps.deploy.outputs.imageUrl }} successfully"
```

## How It Works

1. **Authenticates** with Tower Cloud
2. **Verifies** the container instance exists and is updatable (rejects `provisioning` / `pending` / `updating` / `failed` / `error`)
3. **Resolves registry** — fetches Tower credentials or uses provided ones
4. **Logs into** the registry
5. **Builds** your Docker image for `linux/amd64`
6. **Pushes** the image to the registry
7. **Re-authenticates** (handles token expiry during long builds)
8. **Updates** the container image via the Tower Cloud v1 API — `PATCH /service/container-instance/containers/{name}/image`. The call returns an operation id (`taskId` output) immediately; the rollout itself runs asynchronously on the Tower side. For private external registries, the first deploy saves a per-container pull secret; subsequent deploys reference it by label automatically.

## Inputs

### Always Required

| Input | Description |
|-------|-------------|
| `tower_user` | Tower Cloud username |
| `tower_password` | Tower Cloud password |
| `organization_id` | Tower Cloud organization ID |
| `container_name` | Name of the existing container instance to update |

### Tower Registry

| Input | Required | Description |
|-------|----------|-------------|
| `tcr_name` | Yes | Tower Container Registry name (credentials fetched automatically) |

### External Registry (Docker Hub, GHCR, ACR, GCR, ECR, etc.)

| Input | Required | Description |
|-------|----------|-------------|
| `registry_url` | Yes | Registry hostname (e.g., `docker.io`, `ghcr.io`) — **no `https://`** |
| `registry_username` | Yes | Registry username |
| `registry_password` | Yes | Registry password or access token |

### Optional

| Input | Default | Description |
|-------|---------|-------------|
| `dockerfilePath` | `Dockerfile` | Dockerfile path relative to project root |
| `buildArguments` | — | Docker build arguments (key=value per line) |

## Outputs

| Output | Description |
|--------|-------------|
| `taskId` | Operation id returned by the v1 API. Poll `GET /service/container-instance/operations/{taskId}` to track rollout. |
| `imageUrl` | Full image URL that was built and pushed |

## Image Tagging

Images are automatically tagged:

```
{registry_url}/{github_repo_name}/{container_name}:{short_commit_sha}
```

Example: `my-registry.hyd.cr.tower.cloud/web-portal/my-app:a1b2c3d`

## Prerequisites

1. **Tower Cloud account** — sign up at [portal.dev.tower.cloud](https://portal.dev.tower.cloud)
2. **Container instance** — create via Tower Cloud portal (this action only updates, never creates)
3. **Container registry** — either a Tower registry or any external Docker-compatible registry
4. **Dockerfile** in your repository
5. **GitHub secrets** configured in your app repo

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `authentication failed` | Wrong Tower credentials or org ID | Verify in Tower Cloud portal |
| `container instance not found` | `container_name` doesn't exist | Create it first in Tower Cloud portal |
| `tower registry not found` | `tcr_name` doesn't exist | Create the registry in Tower Cloud portal |
| `registry authentication failed` | Wrong registry credentials | Verify `registry_username` / `registry_password` |
| `cannot reach registry` | Registry URL incorrect or unreachable | Check URL for typos, no `https://` |
| `invalid reference format` | Registry URL has `https://` or uppercase chars | Use hostname only, lowercase |
| `container is provisioning/pending/updating` | Previous deploy still in progress | Wait and retry |
| `container is in failed/error state` | Container has terminal error | Resolve in Tower Cloud portal |
| `NO_EFFECTIVE_CHANGE` | Image reference matches what's already deployed (same commit SHA) | Push a new commit; rebuilds with a new SHA |
| `OPERATION_IN_PROGRESS` | Another operation on this container started after preflight | Wait for it to finish and re-run |
