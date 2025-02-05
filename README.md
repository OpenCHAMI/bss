# Boot Script Service (BSS)

## Summary of Repo 
The Boot Script Service (BSS) provides boot arguments (initrd, kernel arguments, etc.) and Level 2 boot services for static images in HPE Shasta systems. It includes the service implementation, a Swagger API specification (`swagger.yaml`), and tests that verify BSS functionality on live Shasta systems.

## Table of Contents
1. [About/Introduction](#aboutintroduction)  
2. [Overview](#overview)  
3. [Build & Install](#build--install)  
4. [Testing](#testing)  
5. [Running](#running)  
6. [More Reading](#more-reading)  

---

## About/Introduction
This repository contains the code for the HMS Boot Script Service (BSS), a stateless microservice responsible for providing boot parameters and related configurations. It was refactored from the original `hms-netboot` code (including `bootargsd` and other components) created during earlier Redfish demos.

---

## Overview
- **Purpose**: BSS delivers boot arguments, initrd, and kernel arguments to HPE Shasta systems and supports Level 2 boot services for static images.  
- **API Specification**: A `swagger.yaml` file in this repo defines the REST API endpoints for BSS.  
- **Architecture**: Designed to be a minimal, stateless service that focuses solely on providing the necessary boot metadata to nodes in a Shasta environment.

---

## Build & Install
This project uses [GoReleaser](https://goreleaser.com/) to automate releases and embed additional build metadata (commit info, build time, versioning, etc.).

### 1. Environment Variables
Before building, make sure to set the following environment variables to include detailed build metadata:

- **GIT_STATE**: Indicates whether there are uncommitted changes. (`clean` if no changes, `dirty` if there are.)
- **BUILD_HOST**: Hostname of the machine performing the build.
- **GO_VERSION**: The version of Go used.
- **BUILD_USER**: The username of the person or system performing the build.

Example:
```bash
export GIT_STATE=$(if git diff-index --quiet HEAD --; then echo 'clean'; else echo 'dirty'; fi)
export BUILD_HOST=$(hostname)
export GO_VERSION=$(go version | awk '{print $3}')
export BUILD_USER=$(whoami)
```

### 2. Installing GoReleaser
Follow the official [GoReleaser installation instructions](https://goreleaser.com/install/) to set up GoReleaser locally.

### 3. Building Locally with GoReleaser
Use snapshot mode to build locally without releasing:

```bash
goreleaser release --snapshot --clean
```

- The build artifacts (including embedded metadata) will be placed in the `dist/` directory.
- Inspect the resulting binaries to ensure the metadata was correctly embedded.

---

## Testing
### BSS CT Testing
- This repository also produces a `cray-bss-test` container image containing tests to verify BSS on live Shasta systems.  
- The tests can be run via `helm test` as part of the Continuous Test (CT) framework during CSM installs or upgrades.  
- The image version (e.g., `vX.Y.Z`) should match the version of the `cray-bss` image under test, both specified in the Helm chart.

---

## Running
BSS is typically deployed as a container-based microservice (e.g., within a Kubernetes cluster on Shasta). Refer to your environment’s specific Helm chart or deployment documentation to run or upgrade BSS.  

In general:
1. Build or obtain the `cray-bss` container image.
2. Deploy via the Helm chart provided in this repository or through your Shasta deployment process.
3. Verify logs and test endpoints as needed (using the `swagger.yaml` API spec or any standard REST client).

---

## More Reading
- [GoReleaser Documentation](https://goreleaser.com/docs/)  
- [Swagger](https://swagger.io/docs/) – for understanding and testing the `swagger.yaml` specification  
- The original HPE version and internal documents (where applicable) for historical reference  

---

_This README is adapted from the original HPE version with minimal changes to match the open-source release format._
