# Create SMD Redfish endpoints for first xname.
POST http://smd:27779/hsm/v2/Inventory/RedfishEndpoints
{
	"ID": "x0c0s1b0",
	"Name": "x0c0s1b0",
	"Hostname": "x0c0s1b0",
	"Domain": "",
	"FQDN": "",
	"Enabled": true,
	"User": "",
	"Password": "",
	"UseSSDP": true,
	"MacRequired": true,
	"MACAddr": "ae:12:e2:ff:89:9d",
	"IPAddress": "10.254.2.10",
	"RediscoverOnUpdate": true,
	"TemplateID": "string"
}

HTTP 201
Content-Type: application/json
[Asserts]
jsonpath "$[0].URI" == "/hsm/v2/Inventory/RedfishEndpoints/x0c0s1b0"

# Create SMD Redfish endpoints for second xname.
POST http://smd:27779/hsm/v2/Inventory/RedfishEndpoints
{
	"ID": "x0c0s1b1",
	"Name": "x0c0s1b1",
	"Hostname": "x0c0s1b1",
	"Domain": "",
	"FQDN": "",
	"Enabled": true,
	"User": "",
	"Password": "",
	"UseSSDP": true,
	"MacRequired": true,
	"MACAddr": "ae:12:e2:ff:89:9e",
	"IPAddress": "10.254.2.11",
	"RediscoverOnUpdate": true,
	"TemplateID": "string"
}

HTTP 201
Content-Type: application/json
[Asserts]
jsonpath "$[0].URI" == "/hsm/v2/Inventory/RedfishEndpoints/x0c0s1b1"

GET http://smd:27779/hsm/v2/Inventory/RedfishEndpoints

# Response should be:
#
# {
# 	"RedfishEndpoints": [
# 		{
# 			"ID": "x0c0s1b0",
# 			"Type": "NodeBMC",
# 			"Name": "x0c0s1b0",
# 			"Hostname": "x0c0s1b0",
# 			"Domain": "",
# 			"FQDN": "x0c0s1b0",
# 			"Enabled": true,
# 			"User": "",
# 			"Password": "",
# 			"UseSSDP": true,
# 			"MACRequired": true,
# 			"MACAddr": "ae:12:e2:ff:89:9d",
# 			"IPAddress": "10.254.2.10",
# 			"RediscoverOnUpdate": true,
# 			"DiscoveryInfo": {
# 				"LastDiscoveryAttempt": "2023-10-12T17:34:57.645257Z",
# 				"LastDiscoveryStatus": "DiscoveryStarted"
# 			}
# 		},
# 		{
# 			"ID": "x0c0s1b1",
# 			"Type": "NodeBMC",
# 			"Name": "x0c0s1b1",
# 			"Hostname": "x0c0s1b1",
# 			"Domain": "",
# 			"FQDN": "x0c0s1b1",
# 			"Enabled": true,
# 			"User": "",
# 			"Password": "",
# 			"UseSSDP": true,
# 			"MACRequired": true,
# 			"MACAddr": "ae:12:e2:ff:89:9e",
# 			"IPAddress": "10.254.2.11",
# 			"RediscoverOnUpdate": true,
# 			"DiscoveryInfo": {
# 				"LastDiscoveryAttempt": "2023-10-12T17:34:57.646209Z",
# 				"LastDiscoveryStatus": "DiscoveryStarted"
# 			}
# 		}
# 	]
# }
HTTP 200
Content-Type: application/json
[Asserts]
jsonpath "$.RedfishEndpoints" count == 2
jsonpath "$.RedfishEndpoints[*].ID" includes "x0c0s1b0"
jsonpath "$.RedfishEndpoints[*].ID" includes "x0c0s1b1"
jsonpath "$.RedfishEndpoints[*].Hostname" includes "x0c0s1b0"
jsonpath "$.RedfishEndpoints[*].Hostname" includes "x0c0s1b1"
jsonpath "$.RedfishEndpoints[*].MACAddr" includes "ae:12:e2:ff:89:9d"
jsonpath "$.RedfishEndpoints[*].MACAddr" includes "ae:12:e2:ff:89:9e"
jsonpath "$.RedfishEndpoints[*].IPAddress" includes "10.254.2.10"
jsonpath "$.RedfishEndpoints[*].IPAddress" includes "10.254.2.11"
jsonpath "$.RedfishEndpoints[*].RediscoverOnUpdate" includes true
jsonpath "$.RedfishEndpoints[*].RediscoverOnUpdate" includes true
