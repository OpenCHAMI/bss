# Generate a boot script for an xname and return it.
GET http://bss:27778/boot/v1/bootscript?name=x0c0s1b0

# Response should be something like:
#
# #!ipxe
# sleep 10
# chain https://api-gw-service-nmn.local/apis/bss/boot/v1/bootscript?name=x0c0s1b0n0&arch=${buildarch}&ts=1697132097
HTTP 200
