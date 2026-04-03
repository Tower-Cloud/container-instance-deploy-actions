# Tower Cloud Container Deploy Action

Build, push, and deploy a container image to Tower Cloud — in a single step.

Commit code, add this action to your workflow, and your changes are live. Works with any language — your Dockerfile defines how the app is built.

## Usage

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

      - name: Build and deploy container
        uses: tower-cloud/container-instance-deploy-actions@main
        with:
          tower_user: ${{ secrets.TOWER_USER }}
          tower_password: ${{ secrets.TOWER_PASSWORD }}
          organization_id: ${{ secrets.TOWER_ORG_ID }}
          container_name: my-container
          registry_url: ${{ secrets.REGISTRY_URL }}
          registry_username: ${{ secrets.REGISTRY_USERNAME }}
          registry_password: ${{ secrets.REGISTRY_PASSWORD }}
```

## How It Works

1. **Authenticates** with Tower Cloud
2. **Verifies** the container instance exists and is updatable
3. **Logs into** your container registry
4. **Builds** your Docker image for `linux/amd64`
5. **Pushes** the image to your registry
6. **Updates** the container instance with the new image

## Image Tagging Convention

Images are automatically tagged using a deterministic convention — no configuration needed:

```
{registry_url}/{github_repo_name}/{container_name}:{commit_sha}
```

**Example:** If your GitHub repo is `acme/web-portal`, your container instance is `portal-app`, and the registry is `my-registry.hyd.cr.tower.cloud`:

```
my-registry.hyd.cr.tower.cloud/web-portal/portal-app:a1b2c3d4e5f6
```

This ensures:
- Every image is uniquely tied to a commit
- Registry is organized by repo → container
- Full audit trail for rollbacks
- No ambiguity — same commit always produces the same tag

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `dockerfilePath` | No | `Dockerfile` | Dockerfile path relative to project root |
| `tower_user` | Yes | — | Tower Cloud username |
| `tower_password` | Yes | — | Tower Cloud password |
| `organization_id` | Yes | — | Tower Cloud organization ID |
| `container_name` | Yes | — | Name of the existing container instance to update |
| `registry_url` | Yes | — | Container registry URL |
| `registry_username` | Yes | — | Registry username for docker login |
| `registry_password` | Yes | — | Registry password for docker login |
| `buildArguments` | No | — | Docker build arguments (key=value per line) |

## Outputs

| Output | Description |
|--------|-------------|
| `taskId` | Deployment task ID returned by Tower Cloud |
| `imageUrl` | Full image URL that was built and pushed |

## Prerequisites

Before using this action, you **must** complete the following setup:

### 1. Tower Cloud Account
Sign up at [console.tower.cloud](https://console.tower.cloud) and note your **Organization ID**.

### 2. Container Registry (TCR)
Create a container registry via the Tower Cloud portal and note:
- **Registry URL** (e.g., `my-registry.hyd.cr.tower.cloud`)
- **Username** and **Password** for docker login

### 3. Container Instance (TCI)
Create a container instance via the Tower Cloud portal:
- Configure your instance (SKU, ports, environment variables, etc.)
- Note the **instance name** — this is your `container_name`

> This action **only updates** existing container instances. It does not create new ones.

### 4. Dockerfile (yours)
Your repository must contain a `Dockerfile`. The action builds it targeting **linux/amd64** (Tower Cloud cluster architecture).

### 5. GitHub Secrets
Add these secrets to your GitHub repository (`Settings > Secrets and variables > Actions`):

| Secret | Description |
|--------|-------------|
| `TOWER_USER` | Tower Cloud username |
| `TOWER_PASSWORD` | Tower Cloud password |
| `TOWER_ORG_ID` | Tower Cloud Organization ID |
| `REGISTRY_URL` | Registry URL (e.g., `my-registry.hyd.cr.tower.cloud`) |
| `REGISTRY_USERNAME` | Registry username |
| `REGISTRY_PASSWORD` | Registry password |

## Build Arguments

Pass Docker build arguments as multiline key=value pairs:

```yaml
- name: Build and deploy
  uses: tower-cloud/container-instance-deploy-actions@main
  with:
    tower_user: ${{ secrets.TOWER_USER }}
    tower_password: ${{ secrets.TOWER_PASSWORD }}
    organization_id: ${{ secrets.TOWER_ORG_ID }}
    container_name: my-container
    registry_url: ${{ secrets.REGISTRY_URL }}
    registry_username: ${{ secrets.REGISTRY_USERNAME }}
    registry_password: ${{ secrets.REGISTRY_PASSWORD }}
    buildArguments: |
      NODE_ENV=production
      API_URL=https://api.example.com
```

## Using Outputs

```yaml
- name: Build and deploy
  id: deploy
  uses: tower-cloud/container-instance-deploy-actions@main
  with:
    tower_user: ${{ secrets.TOWER_USER }}
    tower_password: ${{ secrets.TOWER_PASSWORD }}
    organization_id: ${{ secrets.TOWER_ORG_ID }}
    container_name: my-container
    registry_url: ${{ secrets.REGISTRY_URL }}
    registry_username: ${{ secrets.REGISTRY_USERNAME }}
    registry_password: ${{ secrets.REGISTRY_PASSWORD }}

- name: Print deployment info
  run: |
    echo "Task ID: ${{ steps.deploy.outputs.taskId }}"
    echo "Image:   ${{ steps.deploy.outputs.imageUrl }}"
```
