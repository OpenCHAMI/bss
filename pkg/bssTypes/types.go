// MIT License
//
// (C) Copyright [2021] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package bssTypes

import (
	"fmt"
	"regexp"

	"github.com/Cray-HPE/hms-xname/xnames"
)

type PhoneHome struct {
	PublicKeyDSA     string `form:"pub_key_dsa" json:"pub_key_dsa" binding:"omitempty"`
	PublicKeyRSA     string `form:"pub_key_rsa" json:"pub_key_rsa" binding:"omitempty"`
	PublicKeyECDSA   string `form:"pub_key_ecdsa" json:"pub_key_ecdsa" binding:"omitempty"`
	PublicKeyED25519 string `form:"pub_key_ed25519" json:"pub_key_ed25519,omitempty"`
	InstanceID       string `form:"instance_id" json:"instance_id" binding:"omitempty"`
	Hostname         string `form:"hostname" json:"hostname" binding:"omitempty"`
	FQDN             string `form:"fqdn" json:"fqdn" binding:"omitempty"`
}

// The main cloud-init struct. Leave the meta-data, user-data, and phone home
// info as generic interfaces as the user defines how much info exists in it
type CloudDataType map[string]interface{}
type CloudInit struct {
	MetaData  CloudDataType `json:"meta-data"`
	UserData  CloudDataType `json:"user-data"`
	PhoneHome PhoneHome     `json:"phone-home,omitempty"`
}

// This is the main data structure used to communicate with the client.  It
// allows the client to set parameters along the with kernel and initrd
// references.  It is also used to return boot info to the user.  The expected
// usage is that one of arrays hosts, macs, or nids is used to  specify the
// hosts for booting.  We could also allow special names for hosts such as
// "compute" or "service" meaning all nodes of those categories, or we
// could introduce an additional property for this type of selection.  We also
// provide a "default" selection which provides a way to supply default
// parameters for any node which is not explicitly configured.
type BootParams struct {
	Hosts     []string  `json:"hosts,omitempty"` // This list of hosts must be xnames
	Macs      []string  `json:"macs,omitempty"`
	Nids      []int32   `json:"nids,omitempty"`
	Params    string    `json:"params,omitempty"`
	Kernel    string    `json:"kernel,omitempty"`
	Initrd    string    `json:"initrd,omitempty"`
	CloudInit CloudInit `json:"cloud-init,omitempty"`
}

// Validate the MACs in the boot parameters
func (bp BootParams) CheckMacs() (err error) {
	if len(bp.Macs) > 0 {
		re := regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9a-fA-F]{2}$`)
		for _, mac := range bp.Macs {
			// If MAC is incorrectly formatted, return error
			if valid := re.MatchString(mac); !valid {
				return fmt.Errorf("invalid MAC address format: %s\n", mac)
			}
		}
	}

	// All MACs are correctly format, return nil error
	return
}

// Validate the xnames in the boot parameters.  They must be of type "Node"
func (bp BootParams) CheckXnames() (err error) {
	for _, xname := range bp.Hosts {
		myXname := xnames.FromString(xname)
		if myXname == nil {
			return fmt.Errorf("invalid xname: %s", xname)
		}
		if myXname.Type() != "Node" {
			return fmt.Errorf("invalid xname type: %s", myXname.Type())
		}
	}
	return nil
}

// The following structures and types all related to the last access information for bootscripts and cloud-init data.

type EndpointType string

const (
	EndpointTypeBootscript EndpointType = "bootscript"
	EndpointTypeUserData   EndpointType = "user-data"
)

var EndpointTypes = []EndpointType{
	EndpointTypeBootscript,
	EndpointTypeUserData,
}

type EndpointAccess struct {
	Name      string       `json:"name"`
	Endpoint  EndpointType `json:"endpoint"`
	LastEpoch int64        `json:"last_epoch"`
}
