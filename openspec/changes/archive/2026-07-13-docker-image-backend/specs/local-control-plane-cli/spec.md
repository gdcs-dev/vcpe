## ADDED Requirements

### Requirement: vcpe build and push support a --backend flag
The `vcpe build` and `vcpe push` commands SHALL accept an optional `--backend <podman|docker>` flag that selects the container runtime for image operations. The default SHALL be `podman`. Passing `--backend` to any other command SHALL be an error.

#### Scenario: Default backend is podman
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml` without `--backend`
- **THEN** the system uses Podman for all image operations, identical to existing behavior

#### Scenario: Docker backend selected for build
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml --backend docker`
- **THEN** the system uses Docker CLI commands for image operations; with default platforms, invokes `docker buildx build --platform linux/amd64,linux/arm64 --tag <image> --push ...` for each buildable service

#### Scenario: Docker multi-arch build embeds push
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml --backend docker` with multiple platforms
- **THEN** the images are pushed to the registry during the build step; no separate `vcpe push` is required

#### Scenario: Docker single-arch build is local only
- **WHEN** an operator runs `vcpe build --manifest deploy.yaml --backend docker --platform linux/arm64`
- **THEN** the system invokes `docker build --tag <image> ...` without `--push`; the image is available only in the local Docker store

#### Scenario: --backend on unsupported command is rejected
- **WHEN** an operator runs `vcpe up --backend docker --manifest deploy.yaml`
- **THEN** the command fails with an error stating `--backend` is only supported for `build` and `push`

### Requirement: pullPolicy compatibility with Docker backend
The help text for `vcpe build --backend docker` SHALL document that manifests using `pullPolicy: build-if-missing` are incompatible with the Docker backend workflow, and that `always-pull` or `missing` must be used so that `vcpe up` pulls the Docker-built images from the registry.

#### Scenario: Help text warns about pullPolicy
- **WHEN** an operator runs `vcpe build --help`
- **THEN** the `--backend` flag description mentions the `pullPolicy` compatibility constraint
