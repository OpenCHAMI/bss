# Add two xnames to SMD.
POST http://smd:27779/hsm/v2/State/Components
{
	"Components": [
		{
			"ID": "x0c0s1b0n0",
			"State": "Ready",
			"Flag": "OK",
			"Enabled": true,
			"SoftwareStatus": "string",
			"Role": "Compute",
			"SubRole": "Worker",
			"NID": 1,
			"Subtype": "string",
			"NetType": "Sling",
			"Arch": "X86",
			"Class": "River"
		},
		{
			"ID": "x0c0s1b1n0",
			"State": "Ready",
			"Flag": "OK",
			"Enabled": true,
			"SoftwareStatus": "string",
			"Role": "Compute",
			"SubRole": "Worker",
			"NID": 2,
			"Subtype": "string",
			"NetType": "Sling",
			"Arch": "X86",
			"Class": "River"
		}
	],
	"Force": true
}

HTTP 204

GET http://smd:27779/hsm/v2/State/Components

# Response should be:
#
# {
#	 "Components": [
#		 {
#			 "ID": "x0c0s1b0n0",
#			 "Type": "Node",
#			 "State": "Ready",
#			 "Flag": "OK",
#			 "Enabled": true,
#			 "SoftwareStatus": "string",
#			 "Role": "Compute",
#			 "SubRole": "Worker",
#			 "NID": 1,
#			 "Subtype": "string",
#			 "NetType": "Sling",
#			 "Arch": "X86",
#			 "Class": "River"
#		 },
#		 {
#			 "ID": "x0c0s1b1n0",
#			 "Type": "Node",
#			 "State": "Ready",
#			 "Flag": "OK",
#			 "Enabled": true,
#			 "SoftwareStatus": "string",
#			 "Role": "Compute",
#			 "SubRole": "Worker",
#			 "NID": 2,
#			 "Subtype": "string",
#			 "NetType": "Sling",
#			 "Arch": "X86",
#			 "Class": "River"
#		 }
#	 ]
# }
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.Components[*].ID" includes "x0c0s1b0n0"
jsonpath "$.Components[*].ID" includes "x0c0s1b1n0"
jsonpath "$.Components[*].NID" includes 1
jsonpath "$.Components[*].NID" includes 2
jsonpath "$.Components[0].Role" == "Compute"
jsonpath "$.Components[0].SubRole" == "Worker"
jsonpath "$.Components[0].Arch" == "X86"
jsonpath "$.Components[0].Enabled" == true
jsonpath "$.Components[1].Role" == "Compute"
jsonpath "$.Components[1].SubRole" == "Worker"
jsonpath "$.Components[1].Arch" == "X86"
jsonpath "$.Components[1].Enabled" == true
