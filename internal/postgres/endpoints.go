// Copyright © 2024 Triad National Security, LLC. All rights reserved.
//
// This program was produced under U.S. Government contract 89233218CNA000001
// for Los Alamos National Laboratory (LANL), which is operated by Triad
// National Security, LLC for the U.S. Department of Energy/National Nuclear
// Security Administration. All rights in the program are reserved by Triad
// National Security, LLC, and the U.S. Department of Energy/National Nuclear
// Security Administration. The Government is granted for itself and others
// acting on its behalf a nonexclusive, paid-up, irrevocable worldwide license
// in this material to reproduce, prepare derivative works, distribute copies to
// the public, perform publicly and display publicly, and to permit others to do
// so.

package postgres

import (
	"fmt"
	"time"

	"github.com/OpenCHAMI/bss/pkg/bssTypes"
)

type EndpointAccess struct {
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	LastEpoch int64  `json:"last_epoch"`
}

// LogEndpointAccess takes a name and an endpoint type and adds a table entry to
// the endpoint_accesses table with the current timestamp.
func (bddb BootDataDatabase) LogEndpointAccess(name string, endpointType bssTypes.EndpointType) (err error) {
	if name == "" {
		err = fmt.Errorf("postgres.LogEndpointAccess: Argument 'name' cannot be empty")
		return
	}
	if endpointType == "" {
		err = fmt.Errorf("postgres.LogEndpointAccess: Argument 'endpointType' cannot be empty")
		return
	}

	ts := time.Now()
	ea := EndpointAccess{
		Name: name,
		Endpoint: string(endpointType),
		LastEpoch: ts.Unix(),
	}

	err = bddb.addEndpointAccess(ea)
	if err != nil {
		err = fmt.Errorf("postgres.LogEndpointAccess: %v", err)
	}

	return
}

func (bddb BootDataDatabase) addEndpointAccess(ea EndpointAccess) (err error) {
	execStr := `INSERT INTO endpoint_access (name, endpoint, last_epoch) VALUES ($1, $2, $3);`
	_, err = bddb.DB.Exec(execStr, ea.Name, ea.Endpoint, ea.LastEpoch)
	if err != nil {
		err = fmt.Errorf("Error executing query to add endpoint access %v: %v", ea, err)
		return
	}

	return
}
