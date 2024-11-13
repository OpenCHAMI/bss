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