// MIT License
//
// Copyright © 2024-2025 Contributors to the OpenCHAMI Project
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/OpenCHAMI/bss/pkg/bssTypes"
)

type EndpointAccess struct {
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	LastEpoch int64  `json:"last_epoch"`
}

// SearchEndpointAccesses takes the name of a node (xname) and a BSS endpoint as
// arguments and returns a slice of EndpointAccess structs representing
// timestamps of when the passed endpoint was accessed for the passed name. If
// endpointType is empty, then all endpoint accesses for the given name are
// returned. If name is empty, then all accesses for the given endpoint are
// returned. If both arguments are empty, then all endpoint accesses for all
// names are returned.
func (bddb BootDataDatabase) SearchEndpointAccesses(name string, endpointType bssTypes.EndpointType) (accesses []bssTypes.EndpointAccess, err error) {
	qstr := `SELECT * FROM endpoint_access`

	// Only construct query with WHERE clause if both arguments are NOT
	// empty.
	if name != "" || endpointType != "" {
		qstr += ` WHERE`
		strs := []string{name, string(endpointType)}
		for first, i := true, 0; i < len(strs); i++ {
			if strs[i] != "" {
				if !first {
					qstr += ` AND`
				}
				switch i {
				case 0:
					qstr += fmt.Sprintf(` name = '%s'`, strs[0])
				case 1:
					qstr += fmt.Sprintf(` endpoint = '%s'`, strs[1])
				}
				first = false
			}
		}
	}
	qstr += `;`
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("postgres.SearchEndpointAccesses: Could not query endpoint access table in boot database: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ea bssTypes.EndpointAccess
		)
		err = rows.Scan(&ea.Name, &ea.Endpoint, &ea.LastEpoch)
		if err != nil {
			err = fmt.Errorf("postgres.SearchEndpointAccesses: Could not scan results into EndpointAccess: %v", err)
			return
		}
		accesses = append(accesses, ea)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("postgres.SearchEndpointAccesses: Could not parse query results: %v", err)
		return
	}

	return
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
		Name:      name,
		Endpoint:  string(endpointType),
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
