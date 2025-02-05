## Build/Install with goreleaser

This project uses [GoReleaser](https://goreleaser.com/) to automate releases and include additional build metadata such as commit info, build time, and versioning. Below is a guide on how to set up and build the project locally using GoReleaser.

### Environment Variables

To include detailed build metadata, ensure the following environment variables are set:

* __GIT_STATE__: Indicates whether there are uncommitted changes in the working directory. Set to clean if the repository is clean, or dirty if there are uncommitted changes.
* __BUILD_HOST__: The hostname of the machine where the build is being performed. 
* __GO_VERSION__: The version of Go used for the build. GoReleaser uses this to ensure consistent Go versioning information.
* __BUILD_USER__: The username of the person or system performing the build.

Set all the environment variables with:
```bash
export GIT_STATE=$(if git diff-index --quiet HEAD --; then echo 'clean'; else echo 'dirty'; fi)
export BUILD_HOST=$(hostname)
export GO_VERSION=$(go version | awk '{print $3}')
export BUILD_USER=$(whoami)
```

### Building Locally with GoReleaser

Once the environment variables are set, you can build the project locally using GoReleaser in snapshot mode (to avoid publishing).


Follow the installation instructions from [GoReleaser’s documentation](https://goreleaser.com/install/).

1. Run GoReleaser in snapshot mode with the --snapshot flag to create a local build without attempting to release it:
  ```bash
  goreleaser release --snapshot --clean
  ```
2.	Check the dist/ directory for the built binaries, which will include the metadata from the environment variables. You can inspect the binary output to confirm that the metadata was correctly embedded.


The rest of this README is unchanged from the HPE version.
__________________________________________________________________

This is the repository for the HMS Boot Script Service (BSS) code.

It includes a swagger.yaml file for the service REST API specification, along with all of the code to implement the stateless
service itself.

This service should contain just what is needed to provide boot arguments (initrd, kargs, etc) and Level 2 boot services for
static images.

This code has been refactored from the old hms-netboot code for bootargsd and associated components created for the Q4 Redfish
and Q1 systems management deep dive demos.

### BSS CT Testing

In addition to the service itself, this repository builds and publishes cray-bss-test images containing tests that verify BSS
on live Shasta systems. The tests are invoked via helm test as part of the Continuous Test (CT) framework during CSM installs
and upgrades. The version of the cray-bss-test image (vX.Y.Z) should match the version of the cray-bss image being tested, both
of which are specified in the helm chart for the service.




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

## 2. Installing GoReleaser
Follow the official [GoReleaser installation instructions](https://goreleaser.com/install/) to set up GoReleaser locally.

## 3. Building Locally with GoReleaser
Use snapshot mode to build locally without releasing:
```bash
goreleaser release --snapshot --clean
The build artifacts (including embedded metadata) will be placed in the dist/ directory.
Inspect the resulting binaries to ensure the metadata was correctly embedded.

# Testing

## BSS CT Testing

This repository also produces a **cray-bss-test** container image containing tests to verify BSS on live Shasta systems.  
The tests can be run via `helm test` as part of the Continuous Test (CT) framework during CSM installs or upgrades.  
The image version (e.g., `vX.Y.Z`) should match the version of the **cray-bss** image under test, both specified in the Helm chart.

---

## Running

BSS is typically deployed as a container-based microservice (e.g., within a Kubernetes cluster on Shasta). Refer to your environment’s specific Helm chart or deployment documentation to run or upgrade BSS.

### In general:

1. **Build or obtain** the `cray-bss` container image.  
2. **Deploy** via the Helm chart provided in this repository or through your Shasta deployment process.  
3. **Verify** logs and test endpoints as needed (using the `swagger.yaml` API spec or any standard REST client).

---

## More Reading

- **[GoReleaser Documentation](https://goreleaser.com/)**  
- **Swagger** – for understanding and testing the `swagger.yaml` specification  
- The original HPE version and internal documents (where applicable) for historical reference


