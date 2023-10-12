# Try to create a group config in BSS.
POST http://bss:27778/boot/v1/bootparameters
{
	"kernel": "https://testkerneluri2.tld",
	"initrd": "https://testinitrduri2.tld",
	"hosts": ["testgroupname"],
	"params": "param3,param4"
}

HTTP 201

GET http://bss:27778/boot/v1/bootparameters

# Response should be:
#
# [
#	 {
#		 "hosts": [
#			 "x0c0s1b0n0",
#			 "x0c0s2b0n0",
#			 "x0c0s3b0n0"
#		 ],
#		 "params": "param1,param2",
#		 "kernel": "https://testkerneluri1.tld",
#		 "initrd": "https://testinitrduri1.tld",
#		 "cloud-init": {
#			 "meta-data": null,
#			 "user-data": null,
#			 "phone-home": {
#				 "pub_key_dsa": "",
#				 "pub_key_rsa": "",
#				 "pub_key_ecdsa": "",
#				 "instance_id": "",
#				 "hostname": "",
#				 "fqdn": ""
#			 }
#		 }
#	 }
# ]
#
# NOTE: Group names are not added new, so "testgroupname" does not appear here.
HTTP 200
Content-Type: application/json; charset=UTF-8
[Asserts]
jsonpath "$[0].hosts" count == 3
jsonpath "$[0].hosts[*]" includes "x0c0s1b0n0"
jsonpath "$[0].hosts[*]" includes "x0c0s2b0n0"
jsonpath "$[0].hosts[*]" includes "x0c0s3b0n0"
jsonpath "$[0].params" == "param1,param2"
jsonpath "$[0].kernel" == "https://testkerneluri1.tld"
jsonpath "$[0].initrd" == "https://testinitrduri1.tld"
