#!/bin/sh
# Run all integration tests.

set -e

# Test add/delete/get boot config by XName.
echo '============================='
echo 'EXECUTING xname UNIT TESTS...'
echo '============================='
/usr/bin/hurl --test "$(dirname $0)"/tests/xname/*.hurl

# Test add/delete/get boot config by MAC address.
echo '============================='
echo 'EXECUTING mac UNIT TESTS...'
echo '============================='
/usr/bin/hurl --test "$(dirname $0)"/tests/mac/*.hurl

# The version of SMD we are using does not support updating/displaying
# NIDs, even though BSS can generate a boot script by NID. Since BSS
# checks SMD to see if a node exists via its NID for this test, it will
# fail. Therefore, this test is commented out for now until SMD handles
# NIDs correctly.
## Test add/delete/get boot config by NID.
#echo '============================='
#echo 'EXECUTING mac UNIT TESTS...'
#echo '============================='
#/usr/bin/hurl --test "$(dirname $0)"/tests/nid/*.hurl

echo '============================='
echo 'ALL UNIT TESTS COMPLETED'
echo '============================='
