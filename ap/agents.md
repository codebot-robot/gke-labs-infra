# ap - Agent Instructions

`ap` (short for **autoproject**) is an opinionated automation tool for gke-labs projects. It uses a "convention over configuration" approach to handle common development tasks.

## AP Roots

- A `.ap` directory marks an **AP root**.
- A repository can contain multiple AP roots, allowing for multiple independent projects within one repo.
- By default, `ap` commands operate on all AP roots found within the git repository.
- Use the `--root` flag to specify a single AP root (e.g., `ap --root autodeploy lint`).

## Recommended Workflow for Agents

When making changes to an AP-managed project, coding agents should follow this verification sequence:

1.  **Generate Code**: Run `ap generate` to update any generated files, CI scripts, or manifests.
2.  **Lint**: Run `ap lint` to ensure code follows project style and quality guidelines.
3.  **Test**:
    -   Run `ap test` for unit and integration tests.
    -   Run `ap e2e` for end-to-end tests.

## Deployment

-   **Command**: `ap deploy`
-   **Images**: `ap` automatically builds Docker images found in the `images/<image-name>` directory.
-   **Kubernetes**: `ap` deploys Kubernetes manifests located in the `k8s` directory.

## Kubernetes Manifests

When writing Kubernetes manifests, follow these guidelines:

-   **Do not specify `imagePullPolicy`**: Kubernetes will automatically set a reasonable default (e.g., `Always` if the tag is `:latest`, `IfNotPresent` otherwise).
-   **Image Naming**: The image name should map to the directory name under `images`. For example, a Dockerfile at `images/foo/Dockerfile` will result in an image named `${IMAGE_PREFIX}/foo`. In your manifest, you should specify the image as `foo` or `foo:latest`. `ap` will handle the prefixing during deployment.
