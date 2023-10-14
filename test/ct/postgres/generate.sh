#!/bin/sh
# Generate an iPXE boot script.
set -e
/usr/bin/hurl "$(dirname $0)"/generate/*.hurl
