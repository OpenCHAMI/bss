# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.28.0] - 2024-01-24

- Added bss-init and postgres migrations
- Moved postgres init code from boot-script-service to bss-init
- Updated docker-compose postgres and bss-debugger files to use one postgres container
- Updated go.mod:
  * Fixed grpc version at v1.29.1 to fix dependency conflict
  * Added github.com/golang-migrate/migrate v4.16.2

## [1.27.2] - 2024-01-19

- Added postgres backend
- Added github-based attestation
- Moved flags to environment variables

## [1.26.0] - 2023-07-06

### Added

- Added retries to destructive bootscript CT tests for hms-nightly-integration runs.

## [1.25.1] - 2023-05-30

### Changed

- CASMCMS-8651: Update example fields in API spec to be links to S3 instead of dear departed ARS.

## [1.25.0] - 2023-05-22

### Added

- CASMHMS-6018: Add support for creating pre-signed URLs for `root=live:` parameters, enabling native dmsquash-live dracut usage.

## [1.24.0] - 2023-03-28

### Changed

- CASMHMS-5812 and CASMHMS-5813: Fixed the endpoint-history API to filter data correctly when using the name and endpoint query parameters.

## [1.23.0] - 2023-01-24

### Changed

- CASMHMS-5894: Minor language linting of API spec; corrected markdown errors in changelog

## [1.22.0] - 2022-11-01

### Changed

- CASMHMS-5796: Created disruptive and destructive Tavern API tests.
- Switch the CT tests to use the hms-simulation-environment.
- Updated Swagger file to reflect how BSS actually works.

## [1.21.0] - 2022-08-19

### Changed

- Added type info in the spire requests when getting a token for the bootscript

## [1.20.0] - 2022-08-08

### Changed

- Added conversion of the metal.server s3 URI in params to a signed URL when creating the boot script.

## [1.19.0] - 2022-07-19

### Changed

- Updated CT tests to hms-test:3.2.0 image to pick up Helm test enhancements and CVE fixes

## [1.18.0] - 2022-06-29

### Changed

- Scrubbed references to HSM v1 in favor of HSM v2.

## [1.17.0] - 2022-06-22

### Changed

- updated CT tests to hms-test:3.1.0 image as part of Helm test coordination

## [1.16.0] - 2022-03-03

### Changed

- converted image builds to be via github actions, updated the image links to be in artifactory.algol60.net
- added a runCT.sh script that can run the tavern tests and smoke tests in a docker-compose environment

## [1.15.0] - 2022-01-07

### Changed

- CASMHMS-4903 Added BSS-Referral-Token to POST and PUT for boot parameters

## [1.14.0] - 2021-12-22

### Changed

- CASMHMS-4540 Improved the performance of getting the bootparamers by name, nid, and mac.

## [1.13.0] - 2021-10-26

### Added

- Enabled tracking of last access time for bootscript and cloud-init (user-data) resources.

## [1.12.0] - 2021-11-30

## Changed

- Enable multi IP support by targeting v2 of HSM.

## [1.11.0] - 2021-10-19

## Changed

- Add priority value to postgres cluster resource

## [1.10.0] - 2021-10-27

### Added

- CASMHMS-5055 - Added BSS CT test RPM.

## [1.9.11] - 2021-09-21

### Changed

- Changed cray-service version to ~6.0.0

## [1.9.10] - 2021-09-08

### Changed

- Changed docker image to run as the user nobody

## [1.9.9] - 2021-08-11

### Changed

- Changed cray-service version to ~2.8.0

## [1.9.8] - 2021-08-09

### Changed

- Added GitHub configuration files.

## [1.9.7] - 2021-08-05

### Changed

- CASMHMS-4943 - Upgraded gopkg.in/yam.v2 to resolve vulnerability.

## [1.9.6] - 2021-08-03

### Added

- Added special priority for BSS pods.

## [1.9.5] - 2021-07-27

### Added

- Changed Stash to GitHub

## [1.9.4] - 2021-07-21

### Added

- Conversion for github
  - Added Makefile
  - Added Jenkinsfile.github

## [1.9.3] - 2021-07-12

### Security

- CASMHMS-4933 - Updated base container images for security updates.

## [1.9.0] - 2021-06-07

## Changed

- Created release branch for CSM 1.2

## [1.8.0] - 2021-06-07

## Changed

- Created release branch for CSM 1.1

## [1.7.6] - 2021-04-20

## Changed

- Updated the BSS dumpstate CT test case for HSM locking.

## [1.7.5] - 2021-04-15

## Changed

- Removed blank IP from preventing BSS from recording an EthernetInterface defined MAC in its list of MACs for a node.

## [1.7.4] - 2021-04-06

## Changed

- Updated Dockerfile to pull base images from Artifactory instead of DTR.

## [1.7.3] - 2021-02-03

## Changed

- Added User-Agent headers to all outbound HTTP requests.

## [1.7.2] - 2021-01-26

## Changed

- CASMHMS-4459: Added logic for any MACs found in the EthernetInterfaces table belonging to a component to be added to that component so subsequent bootscript queries will make the proper association despite HSM not having discovered that MAC in Redfish.

## [1.7.1] - 2021-01-24

### Changed

- CASMINST-1074: Use HTTP S3 endpoint instead of HTTPS. The HTTPS endpoint was causing iPXE to be unable to fetch boot artifacts from S3. The switch to using http will enable iPXE to fetch these boot artifacts.

## [1.7.0] - 2021-01-14

### Changed

- Updated license file.


## [1.6.0] - 2020-12-15

### Changed

- CASMINST-597 - Refactored common structs out into package directory for importing into other projects.

## [1.5.4] - 2020-12-02

### Changed

- CASMHMS-4242 - Update PATCH processing of boot parameters to go into
  the meta-data and user-data and patch individual keys within those
  structures.

## [1.5.3] - 2020-11-24

### Changed

- CASMHMS-3841 - Update helm chart to obtain S3 endpoint from bss-s3-credentials

## [1.5.2] - 2020-11-18

### Changed

- CASMHMS-3878 - Updates to support Spire join token service
- CASMHMS-4219 - Update hms-s3 package version.
- CASMINST-206 - Remove "management" black-listing.

## [1.5.1] - 2020-11-10

### Changed

- CASMHMS-4209 - Updated Jenkinsfile to use the csm product stream.
- CASMHMS-4105 - Resolve libcrypto vulnerability.

## [1.5.0] - 2020-10-29

### Added

- MTL-1000 - Added cloud-init feature

### Changed

- MTL-1000 - Requests to kernel params will now include cloud-init server information

## [1.4.1] - 2020-10-02

### Changed

- CASMHMS-4078 - Update version to pull updated cray-service base charts version 2.0.1

## [1.4.0] - 2020-09-15

### Changed

- CASMCLOUD-1023
  These are changes to charts in support of:
  - moving to Helm v1/Loftsman v1
  - the newest 2.x cray-service base chart
    - upgraded to support Helm v3
    - modified containers/init containers, volume, and persistent volume claim value definitions to be objects instead of arrays
  - the newest 0.2.x cray-jobs base chart upgraded to support Helm v3

## [1.3.5] - 2020-08-18

### Changed

- CASMHMS-2731 - Refactor BSS to use new common repos instead of hms-common.

## [1.3.4] - 2020-08-10

### Changed

- CASMHMS-3889 - Change smd connectivity check to consume SMD output and Close response body
                 in order to prevent leaking smd connections, causing resource problems.

## [1.3.3] - 2020-07-30

### Changed

- CASMHMS-3829 - Adjust resources for the BSS container.

## [1.3.2] - 2020-07-13

### Changed

- CASMHMS-2406 - Update responses to methods not allowed.
- CASMHMS-3673 - Update logging of errors when processing /boot/v1/service requests.

## [1.3.1] - 2020-06-30

### Added

- CASMHMS-3626 - Updated BSS CT smoke test with new API test cases.

## [1.3.0] - 2020-06-26

### Changed

- CASMHMS-3660 - change base chart to 1.11.1 for ETCD improvements

## [1.2.8] - 2020-06-12

### Changed

- CASMHMS-3568 - change base chart to 1.8 for ETCD improvements

## [1.2.7] - 2020-06-08

### Changed

- CASMHMS-1894 - Update BSS to use state change notification in order to keep in sync with SMD.

## [1.2.6] - 2020-06-03

### Changed

- CASMHMS-3530 - Updated BSS dumpstate CT test case to support optional HSM SoftwareStatus field.

## [1.2.5] - 2020-05-22

### Changed

- Bumped cray-service chart version to 1.5.3. This new chart includes the the proper fix for the immutable job issue with the wait-for-postgres job.

## [1.2.4] - 2020-05-13

### Changed

- CASMHMS-3267 - Bumped cray-service chart version to 1.4.0 to support online upgrade and rollback. Improved the ETCD connection retry loop to be more configurable. The environment variables `ETCD_RETRY_COUNT` and `ETCD_RETRY_WAIT` can be used to control the retry count and wait period when initially connecting to ETCD.

## [1.2.3] - 2020-05-06

### Changed

- CASMHMS-332 - Set replicaCount to 3 for k8s deployment.

## [1.2.2] - 2020-04-27

### Changed

- CASMHMS-2951 - Updated hms-bss to use trusted baseOS.

## [1.2.1] - 2020-03-30

### Changed

- CASMHMS-3211 - Disable dumpstate CT test case for bad MAC data issue CASMHMS-3216.

## [1.2.0] - 2020-03-27

### Changed

- Update cray-service dependency to use the 1.3.0 version

## [1.1.8] - 2020-03-02

### Changed

- Update cray-service dependency to use the 1.2.0 version

## [1.1.7] - 2020-02-26

### Changed

- Removed extraneous quotes from URL in kernel command of generated iPXE script

## [1.1.6] - 2020-02-26

### Added

- Support for S3 URLs, which are converted to presigned URLs

## [1.1.5] - 2020-01-31

### Added

- CASMHMS-2473 - Added initial set of BSS Tavern API tests for CT framework.

## [1.1.3] - 2019-05-14

### Changed

- Changed the protocol being used for chained requests in generated boot scripts to use https by default, and allow this to be changed via an environment variable, BSS_CHAIN_PROTO.

## [1.1.0] - 2019-05-14

### Changed

- Moved folders around to better fit with new layout policy.
- Targeted v1.1.0 of `hms-common` which no longer includes SMD packages, so targeted the SMD repo for those now.

## [1.0.0] - 2019-05-13

### Added

- This is the initial release. It contains everything that was in `hms-services` at the time with the major exception of being `go mod` based now.

### Changed

### Deprecated

### Removed

### Fixed

### Security
