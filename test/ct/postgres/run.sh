#!/bin/sh
# Run all integration tests.

set -e

"$(dirname $0)"/add.sh
"$(dirname $0)"/generate.sh
"$(dirname $0)"/delete.sh
