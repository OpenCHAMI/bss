# Get list of components from SMD.
GET http://smd:27779/hsm/v2/State/Components

# Response should be:
#
# {
#	 "Components": [
#		 {
#			 "ID": "x0c0s1b0",
#			 "Type": "Node",
#			 "Enabled": false
#		 },
#		 {
#			 "ID": "x0c0s1b1",
#			 "Type": "Node",
#			 "Enabled": false
#		 }
#	 ]
# }
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.Components[*].ID" includes "x0c0s1b0"
jsonpath "$.Components[*].ID" includes "x0c0s1b1"
jsonpath "$.Components[0].Type" == "Node"
jsonpath "$.Components[0].Enabled" == false # This should be set to true eventually.
jsonpath "$.Components[1].Type" == "Node"
jsonpath "$.Components[1].Enabled" == false # This should be set to true eventually.
