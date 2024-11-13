# MIT License
#
# (C) Copyright [2018-2021] Hewlett Packard Enterprise Development LP
#
# Permission is hereby granted, free of charge, to any person obtaining a
# copy of this software and associated documentation files (the "Software"),
# to deal in the Software without restriction, including without limitation
# the rights to use, copy, modify, merge, publish, distribute, sublicense,
# and/or sell copies of the Software, and to permit persons to whom the
# Software is furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included
# in all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
# THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
# OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
# OTHER DEALINGS IN THE SOFTWARE.

FROM cgr.dev/chainguard/wolfi-base
EXPOSE 27778
STOPSIGNAL SIGTERM

RUN apk add --no-cache tini

# URL of SMD (needed for confirming node existence).
ENV HSM_URL=http://smd:27779
# URL of notification daemon.
ENV NFD_URL=http://cray-hmnfd

# Address of cloud-init server to be added to kernel parameters.
ENV BSS_ADVERTISE_ADDRESS=localhost

# Use an insecure (no TLS) connection to Etcd or Postgres (whichever is used).
# WARNING: Our containers currently do not have certificates set up correctly
#          to allow for https connections to other containers.  Therefore, we
#          will use an insecure connection.  This needs to be corrected before
#          release.  Once the certificates are properly set up, this can be
#          set to false.
ENV BSS_INSECURE=true

# Seconds to sleep before retrying boot in iPXE boot script if boot fails
# for some reason.
ENV BSS_RETRY_DELAY=30
# Seconds to sleep before retrying iPXE boot in default boot script.
# This is for when Etcd is being used and we need to wait for node state to
# be updated before trying to boot.
ENV BSS_HSM_RETRIEVAL_DELAY=10

# Other potentially useful variables with default values:
#
# iPXE server to point nodes to.
# BSS_IPXE_SERVER=api-gw-service-nmn.local
#
# Protocol to use for chains in boot scripts.
# BSS_CHAIN_PROTO=https
#
# The name of BSS.
# BSS_SERVICE_NAME=boot-script-service
#
# Where BSS should listen to requests.
# BSS_HTTP_LISTEN=:27778
#
# URL to listen for notifications on.
# BSS_ENDPOINT_HOST=""
#
# URL of SPIRE token service (not necessary to run BSS).
# SPIRE_TOKEN_URL=https://spire-tokens.spire:54440
#
# URL of JSON Web Key Set (JWKS) server to use for verifying JWTs.
# When this is set, JWT authentication is enabled. Otherwise, it
# is disabled.
# BSS_JWKS_URL=""
#
# Base URL of the Oauth2 server admin endpoints to use for client authorizations
# when JWT authentication is enabled. This is used to authorize BSS via a client
# credentials grant to be able to communicate with protected SMD endpoints when
# it is queried for a boot script.
# BSS_OAUTH2_ADMIN_BASE_URL=http://127.0.0.1:4445
#
# Base URL of the OAuth2 server public endpoints to use for non-admin requests
# like a client (e.g. BSS) requesting an access token after it has been
# authorized.
# BSS_OAUTH2_USER_BASE_URL=http://127.0.0.1:4444

# Etcd variables with default values:
#
# Base URL of KV datastore (could alternatively specify by ETCD_HOST and
# ETCD_PORT below). By default, the in-memory datastore is used (all variables
# empty).
# DATASTORE_BASE=""
# ETCD_HOST=""
# ETCD_PORT=""
#
# Number of times to attempt connection to datastore before giving up.
# ETCD_RETRY_COUNT=10
#
# Seconds between connection attempts.
# ETCD_RETRY_WAIT=5

# Postgres variables with default values:
#
# Configure BSS to use Postgres instead of Etcd.
# Required to be true for the following variables to be used. Note that Postgres
# is disabled and Etcd is enabled by default.
# BSS_USESQL=false
#
# Enable BSS debugging messages.
# BSS_DEBUG=false
#
# Location of Postgres server.
# BSS_DBHOST=localhost
#
# Port of Postgres server.
# BSS_DBPORT=5432
#
# Name of BSS database in Postgres to connect to.
# BSS_DBNAME=bssdb
#
# Database options to pass to Postgres.
# BSS_DBOPTS=""
#
# Postgres username.
# BSS_DBUSER=bssuser
#
# Postgres user password.
# BSS_DBPASS=bssuser
#
# How many times to try to connect to Postgres before giving up.
# BSS_SQL_RETRY_COUNT=10
#
# Number of seconds between connection attempts to Postgres.
# BSS_SQL_RETRY_WAIT=5

# Include curl in the final image.
RUN set -ex \
    && apk -U upgrade \
    && apk add --no-cache curl

# Get the boot-script-service and bss-init from the builder stage.
COPY boot-script-service /usr/local/bin/
COPY bss-init /usr/local/bin/
COPY migrations/* /migrations/

# nobody 65534:65534
USER 65534:65534

# Set up the command to start the service.
CMD /usr/local/bin/boot-script-service

ENTRYPOINT ["/sbin/tini", "--"]
