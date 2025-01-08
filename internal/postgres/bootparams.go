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
	"database/sql"
	"fmt"
	"strings"

	"github.com/OpenCHAMI/bss/pkg/bssTypes"
	"github.com/docker/distribution/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/Cray-HPE/hms-xname/xnames"
	_ "github.com/lib/pq"
)

type Node struct {
	Id      string `json:"id"`
	BootMac string `json:"boot_mac,omitempty"`
	Xname   string `json:"xname,omitempty"`
	Nid     int32  `json:"nid,omitempty"`
}

type BootConfig struct {
	Id        string `json:"id"`                   // UUID of this boot configuration
	KernelUri string `json:"kernel_uri"`           // URI to kernel image
	InitrdUri string `json:"initrd_uri,omitempty"` // URI to initrd image
	Cmdline   string `json:"cmdline,omitempty"`    // boot parameters associated with this image
}

type BootGroup struct {
	Id           string `json:"id"`
	BootConfigId string `json:"boot_config_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
}

type BootGroupAssignment struct {
	BootGroupId string `json:"boot_group_id"`
	NodeId      string `json:"node_id"`
}

type BootDataDatabase struct {
	DB *sqlx.DB
	// TODO: Utilize cache.
	//ImageCache map[string]Image
}

// A helper struct to have a boot group and its corresponding
// boot config in the same place.
type bgbc struct {
	Bg BootGroup
	Bc BootConfig
}

// NewNode creates a new Node and populates it with the boot MAC address (converts to lower case),
// XName, and NID specified.  Before returning the Node, its ID is populated with a new unique
// identifier.
func NewNode(mac, xname string, nid int32) (n Node) {
	n.Id = uuid.Generate().String()
	n.BootMac = strings.ToLower(mac)
	n.Xname = xname
	n.Nid = nid
	return n
}

// NewBootGroup creates a new BootGroup and populates it with the specified boot config ID, name,
// and description, as well as populates its ID with a unique identifier. The new BootGroup is
// returned.
func NewBootGroup(bcId, bgName, bgDesc string) (bg BootGroup) {
	bg.Id = uuid.Generate().String()
	bg.BootConfigId = bcId
	bg.Name = bgName
	bg.Description = bgDesc
	return bg
}

// NewBootConfig creates a new BootConfig and populates it with kernel and initrd images, as well
// as additional boot parameters, generates a unique ID, and returns the new BootConfig. If
// kernelUri is blank, an error is returned.
func NewBootConfig(kernelUri, initrdUri, cmdline string) (bc BootConfig, err error) {
	if kernelUri == "" {
		err = fmt.Errorf("kernel URI cannot be blank")
		return BootConfig{}, err
	}
	bc.KernelUri = kernelUri
	bc.InitrdUri = initrdUri
	bc.Cmdline = cmdline
	bc.Id = uuid.Generate().String()
	return bc, err
}

// NewBootGroupAssignment creates a new BootGroupAssignment and populates it with the boot group id
// and node ID specified, returning the BootGroupAssignment that got created. If either bgId or
// nodeId is blank, an error is returned.
func NewBootGroupAssignment(bgId, nodeId string) (bga BootGroupAssignment, err error) {
	if bgId == "" || nodeId == "" {
		err = fmt.Errorf("boot group ID or node MAC cannot be blank")
		return BootGroupAssignment{}, err
	}
	bga.BootGroupId = bgId
	bga.NodeId = nodeId
	return bga, err
}

// addNodes adds one or more Nodes to the nodes table without checking if they exist. If an error
// occurs with the query execution, that error is returned.
func (bddb BootDataDatabase) addNodes(nodes []Node) (err error) {
	execStr := `INSERT INTO nodes (id, boot_mac, xname, nid) VALUES ($1, $2, $3, $4);`
	for _, n := range nodes {
		_, err = bddb.DB.Exec(execStr, n.Id, n.BootMac, n.Xname, n.Nid)
		if err != nil {
			err = fmt.Errorf("error executing query to add node %v: %w", n, err)
			return err
		}
	}
	return err
}

// addBootConfigs adds a list of BootConfigs to the boot_configs table without checking if they
// exist. If an error occurs with the query execution, that error is returned.
func (bddb BootDataDatabase) addBootConfigs(bc []BootConfig) (err error) {
	execStr := `INSERT INTO boot_configs (id, kernel_uri, initrd_uri, cmdline) VALUES ($1, $2, $3, $4);`
	for _, b := range bc {
		_, err := bddb.DB.Exec(execStr, b.Id, b.KernelUri, b.InitrdUri, b.Cmdline)
		if err != nil {
			err = fmt.Errorf("error executing query to add boot configs: %w", err)
			return err
		}
	}
	return err
}

// addBootGroups adds a list of BootGroups to the boot_groups table without checking if they exist.
// If an error occurs with the query execution, that error is returned.
func (bddb BootDataDatabase) addBootGroups(bg []BootGroup) (err error) {
	execStr := `INSERT INTO boot_groups (id, boot_config_id, name, description) VALUES ($1, $2, $3, $4);`
	for _, b := range bg {
		_, err = bddb.DB.Exec(execStr, b.Id, b.BootConfigId, b.Name, b.Description)
		if err != nil {
			err = fmt.Errorf("error executing query to add boot groups: %w", err)
			return err
		}
	}
	return err
}

// addBootGroupAssignments adds a list of BootGroupAssignments to the boot_group_assignments table
// without checking if they exist. If an error occurs with the query execution, that error is
// returned.
func (bddb BootDataDatabase) addBootGroupAssignments(bga []BootGroupAssignment) (err error) {
	execStr := `INSERT INTO boot_group_assignments (boot_group_id, node_id) VALUES ($1, $2);`
	for _, b := range bga {
		_, err = bddb.DB.Exec(execStr, b.BootGroupId, b.NodeId)
		if err != nil {
			err = fmt.Errorf("error executing query to add boot group assignments: %w", err)
			return err
		}
	}
	return err
}

// updateNodeAssignment updates the boot group assignment(s) of one or more nodes to a different
// boot group (and thus, a different boot config).
func (bddb BootDataDatabase) updateNodeAssignment(nodeIds []string, bgId string) (err error) {
	if len(nodeIds) == 0 {
		err = fmt.Errorf("no node IDs specified")
		return err
	}
	if len(bgId) == 0 {
		err = fmt.Errorf("no boot group ID specified")
		return err
	}

	execStr := `UPDATE boot_group_assignments bga SET boot_group_id = $1` +
		` WHERE node_id IN ` + stringSliceToSql(nodeIds) +
		`;`
	_, err = bddb.DB.Exec(execStr, bgId)
	if err != nil {
		err = fmt.Errorf("error executing update on boot group assignments: %w", err)
		return err
	}

	return err
}

// GetNodes returns a list of all nodes in the nodes table within bddb.
func (bddb BootDataDatabase) GetNodes() ([]Node, error) {
	nodeList := []Node{}
	qstr := `SELECT * FROM nodes;`
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not query node table in boot database: %w", err)
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("could not scan results into Node: %w", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse query results: %w", err)
		return nodeList, err
	}

	return nodeList, err
}

// CheckNodeExistence takes takes a slice of MAC addresses, a slice of XNames, and a slice of NIDs
// and checks to see if the nodes corresponding to them exist in the nodes table. Those that do
// exist are added to an existing Node slice. MAC addresses, XNames, and NIDs that do not correspond
// to any existing nodes are added to a corresponding slice of non-existing MACs/XNames/NIDs. The
// slices of existing nodes, nonexisting MAC addresses, nonexisting XNames, and nonexisting NIDs are
// returned. If an error occurs when querying the database, it is returned.
func (bddb BootDataDatabase) CheckNodeExistence(macs, xnames []string, nids []int32) (existingNodes []Node, nonExistingMacs, nonExistingXnames []string, nonExistingNids []int32, err error) {
	// Get nodes that exist.
	existingNodes, err = bddb.GetNodesByItems(macs, xnames, nids)
	if err != nil {
		err = fmt.Errorf("error checking node existence for macs=%v xnames=%v nids=%v: %w", macs, xnames, nids, err)
		return existingNodes, nonExistingMacs, nonExistingXnames, nonExistingNids, err
	}

	// Create three maps:
	//   1. mac -> Node
	//   2. xname -> Node
	//   3. nid -> Node
	// Iterate through each Node in the existing node list and add each's mac/xname/nid
	// to the corresponding map. These will be used to determine whether macs/xnames/nids
	// that were passed exist or not.
	macToNode := make(map[string]Node)
	xnameToNode := make(map[string]Node)
	nidToNode := make(map[int32]Node)
	for _, n := range existingNodes {
		macToNode[n.BootMac] = n
		xnameToNode[n.Xname] = n
		nidToNode[n.Nid] = n
	}

	// Iterate through each slice of mac/xname/nid and categorize each's existence.
	for _, m := range macs {
		if _, ok := macToNode[m]; !ok {
			nonExistingMacs = append(nonExistingMacs, m)
		}
	}
	for _, x := range xnames {
		if _, ok := xnameToNode[x]; !ok {
			nonExistingXnames = append(nonExistingXnames, x)
		}
	}
	for _, n := range nids {
		if _, ok := nidToNode[n]; !ok {
			nonExistingNids = append(nonExistingNids, n)
		}
	}

	return existingNodes, nonExistingMacs, nonExistingXnames, nonExistingNids, err
}

// GetNodesByItems queries the nodes table for any Nodes that has an XName, MAC address, or NID that
// matches any in macs, xnames, or nids. Any matches found are returned. Otherwise, an empty Node
// list is returned. If no macs, xnames, or nids are specified, all nodes are returned.
func (bddb BootDataDatabase) GetNodesByItems(macs, xnames []string, nids []int32) ([]Node, error) {
	nodeList := []Node{}

	// If no items are specified, get all nodes.
	if len(macs) == 0 && len(xnames) == 0 && len(nids) == 0 {
		return bddb.GetNodes()
	}

	qstr := `SELECT * FROM nodes WHERE`
	lengths := []int{len(macs), len(xnames), len(nids)}
	for first, i := true, 0; i < len(lengths); i++ {
		if lengths[i] > 0 {
			if !first {
				qstr += ` OR`
			}
			switch i {
			case 0:
				// Ignore case when searching by MAC.
				var macsLower []string
				for _, mac := range macs {
					macsLower = append(macsLower, strings.ToLower(mac))
				}
				qstr += fmt.Sprintf(` boot_mac IN %s`, stringSliceToSql(macsLower))
			case 1:
				qstr += fmt.Sprintf(` xname IN %s`, stringSliceToSql(xnames))
			case 2:
				qstr += fmt.Sprintf(` nid IN %s`, int32SliceToSql(nids))
			}
			first = false
		}
	}
	qstr += `;`
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not query node table in boot database: %w", err)
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("could not scan results into Node: %w", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse query results: %w", err)
		return nodeList, err
	}

	return nodeList, err
}

// GetNodesByBootGroupId returns a slice of Nodes that are a member of the BootGroup with an ID of
// bgId. If an error occurs during the query or scanning, an error is returned.
func (bddb BootDataDatabase) GetNodesByBootGroupId(bgId string) ([]Node, error) {
	nodeList := []Node{}

	// If no boot group ID is specified, get all nodes.
	if bgId == "" {
		return bddb.GetNodes()
	}

	qstr := `SELECT n.id, n.boot_mac, n.xname, n.nid FROM nodes AS n` +
		` LEFT JOIN boot_group_assignments AS bga ON n.id=bga.node_id` +
		fmt.Sprintf(` WHERE bga.boot_group_id='%s';`, bgId)
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetNodesByBootGroupID: unable to query database: %w", err)}
		return nodeList, err
	}

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = ErrPostgresGet{Err: fmt.Errorf("GetNodesByBootGroupId: could not scan SQL result: %w", err)}
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetNodesByBootGroupId: could not parse query results: %w", err)}
		return nodeList, err
	}

	return nodeList, err
}

// GetBootConfigsAll returns a slice of all BootGroups and a slice of all BootConfigs in the
// database, as well as the number of items in these slices (each BootGroup corresponds to a
// BootConfig, so these slices have the same number of items). If an error occurs with the query or
// scanning of the query results, an error is returned.
func (bddb BootDataDatabase) GetBootConfigsAll() ([]BootGroup, []BootConfig, int, error) {
	bgResults := []BootGroup{}
	bcResults := []BootConfig{}
	numResults := 0

	qstr := "SELECT bg.id, bg.name, bg.description, bc.id, bc.kernel_uri, bc.initrd_uri, bc.cmdline FROM boot_groups AS bg" +
		" LEFT JOIN boot_configs AS bc" +
		" ON bg.boot_config_id=bc.id" +
		";"
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootConfigsAll: unable to query database: %w", err)}
		return bgResults, bcResults, numResults, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var (
			bg BootGroup
			bc BootConfig
		)
		err = rows.Scan(&bg.Id, &bg.Name, &bg.Description,
			&bc.Id, &bc.KernelUri, &bc.InitrdUri, &bc.Cmdline)
		if err != nil {
			err = ErrPostgresGet{Err: fmt.Errorf("GetBootConfigsAll: could not scan SQL result: %w", err)}
			return bgResults, bcResults, numResults, err
		}
		bg.BootConfigId = bc.Id

		bgResults = append(bgResults, bg)
		bcResults = append(bcResults, bc)
		numResults++
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootConfigsAll: could not parse query results: %w", err)}
		return bgResults, bcResults, numResults, err
	}

	return bgResults, bcResults, numResults, err
}

// GetBootConfigsByItems returns a slice of BootGroups and a slice of BootConfigs that match the
// passed kernel URI, initrd URI, and parameters, as well as the number of results that were
// returned (each BootGroup corresponds to a BootConfig, so these slices have the same number of
// items). If an error occurs with the query or scanning of the query results, an error is returned.
func (bddb BootDataDatabase) GetBootConfigsByItems(kernelUri, initrdUri, cmdline string) ([]BootGroup, []BootConfig, int, error) {
	// If no items are specified, get all boot configs, mapped by boot group.
	if kernelUri == "" && initrdUri == "" && cmdline == "" {
		return bddb.GetBootConfigsAll()
	}

	bgResults := []BootGroup{}
	bcResults := []BootConfig{}
	numResults := 0

	qstr := "SELECT bg.id, bg.name, bg.description, bc.id, bc.kernel_uri, bc.initrd_uri, bc.cmdline FROM boot_groups AS bg" +
		" LEFT JOIN boot_configs AS bc" +
		" ON bg.boot_config_id=bc.id" +
		" WHERE"
	lengths := []int{len(kernelUri), len(initrdUri), len(cmdline)}
	for first, i := true, 0; i < len(lengths); i++ {
		if lengths[i] > 0 {
			if !first {
				qstr += " OR"
			}
			switch i {
			case 0:
				qstr += fmt.Sprintf(" kernel_uri='%s'", kernelUri)
			case 1:
				qstr += fmt.Sprintf(" initrd_uri='%s'", initrdUri)
			case 2:
				qstr += fmt.Sprintf(" cmdline='%s'", cmdline)
			}
			first = false
		}
	}
	qstr += ";"
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootConfigsAll: unable to query database: %w", err)}
		return bgResults, bcResults, numResults, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var (
			bg BootGroup
			bc BootConfig
		)
		err = rows.Scan(&bg.Id, &bg.Name, &bg.Description,
			&bc.Id, &bc.KernelUri, &bc.InitrdUri, &bc.Cmdline)
		if err != nil {
			err = ErrPostgresGet{Err: fmt.Errorf("GetBootConfigsAll: could not scan SQL result: %w", err)}
			return bgResults, bcResults, numResults, err
		}
		bg.BootConfigId = bc.Id

		bgResults = append(bgResults, bg)
		bcResults = append(bcResults, bc)
		numResults++
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootConfigsAll: could not parse query results: %w", err)}
		return bgResults, bcResults, numResults, err
	}

	return bgResults, bcResults, numResults, err
}

// Obtain a map of nodes mapping to their corresponding boot group and boot config.
func (bddb BootDataDatabase) getNodesWithConfigs(macs, xnames []string, nids []int32) (map[Node]bgbc, error) {
	var err error
	nToBgbc := make(map[Node]bgbc)
	qstr := `SELECT n.id, n.boot_mac, n.xname, n.nid,` +
		` bg.id, bg.name, bg.description,` +
		` bc.id, bc.kernel_uri, bc.initrd_uri, bc.cmdline` +
		` FROM nodes AS n` +
		` JOIN boot_group_assignments AS bga ON n.id=bga.node_id` +
		` JOIN boot_groups AS bg ON bga.boot_group_id=bg.id` +
		` JOIN boot_configs AS bc ON bg.boot_config_id=bc.id`
	if len(macs) > 0 || len(xnames) > 0 || len(nids) > 0 {
		qstr += ` WHERE`
		lengths := []int{len(macs), len(xnames), len(nids)}
		for first, i := true, 0; i < len(lengths); i++ {
			if lengths[i] > 0 {
				if !first {
					qstr += ` OR`
				}
				switch i {
				case 0:
					qstr += fmt.Sprintf(` boot_mac IN %s`, stringSliceToSql(macs))
				case 1:
					qstr += fmt.Sprintf(` xname IN %s`, stringSliceToSql(xnames))
				case 2:
					qstr += fmt.Sprintf(` nid IN %s`, int32SliceToSql(nids))
				}
				first = false
			}
		}
	}
	qstr += `;`

	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not query nodes with boot configs: %w", err)
		return nToBgbc, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			n   Node
			cfg bgbc
		)
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid,
			&cfg.Bg.Id, &cfg.Bg.Name, &cfg.Bg.Description,
			&cfg.Bc.Id, &cfg.Bc.KernelUri, &cfg.Bc.InitrdUri, &cfg.Bc.Cmdline)
		if err != nil {
			err = fmt.Errorf("could not scan query results: %w", err)
			return nToBgbc, err
		}
		cfg.Bg.BootConfigId = cfg.Bc.Id

		nToBgbc[n] = cfg
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("error parsing query results: %w", err)
		return nToBgbc, err
	}

	return nToBgbc, err
}

// Obtain a map of boot groups and boot configs mapping to the list of nodes they correspond to.
func (bddb BootDataDatabase) getConfigsWithNodes(nodeIds []string) (map[bgbc][]Node, error) {
	var err error
	bgbcToN := make(map[bgbc][]Node)

	if len(nodeIds) == 0 {
		err = fmt.Errorf("no node IDs specified")
		return bgbcToN, err
	}

	qstr := `SELECT bg.id FROM boot_groups AS bg` +
		` JOIN boot_group_assignments AS bga ON bg.id=bga.boot_group_id` +
		` WHERE bga.node_id IN ` + stringSliceToSql(nodeIds) +
		`;`

	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not query boot configs and groups from node IDs: %w", err)
		return bgbcToN, err
	}
	defer rows.Close()

	var bgIds []string
	for rows.Next() {
		var bgId string
		err = rows.Scan(&bgId)
		if err != nil {
			err = fmt.Errorf("could not scan query results: %w", err)
			return bgbcToN, err
		}

		bgIds = append(bgIds, bgId)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("error parsing query results: %w", err)
		return bgbcToN, err
	}
	rows.Close()

	qstr = `SELECT bg.id, bg.name, bg.description,` +
		` bc.id, bc.kernel_uri, bc.initrd_uri, bc.cmdline,` +
		` n.id, n.boot_mac, n.xname, n.nid` +
		` FROM boot_groups AS bg` +
		` JOIN boot_configs AS bc ON bg.boot_config_id=bc.id` +
		` JOIN boot_group_assignments AS bga ON bg.id=bga.boot_group_id` +
		` JOIN nodes AS n ON bga.node_id=n.id` +
		` WHERE bg.id IN ` + stringSliceToSql(bgIds) +
		`;`

	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not query boot configs with nodes: %w", err)
		return bgbcToN, err
	}

	for rows.Next() {
		var (
			cfg bgbc
			n   Node
		)
		err = rows.Scan(&cfg.Bg.Id, &cfg.Bg.Name, &cfg.Bg.Description,
			&cfg.Bc.Id, &cfg.Bc.KernelUri, &cfg.Bc.InitrdUri, &cfg.Bc.Cmdline,
			&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("could not scan query results: %w", err)
			return bgbcToN, err
		}
		cfg.Bg.BootConfigId = cfg.Bc.Id

		if tmpNodeList, ok := bgbcToN[cfg]; !ok {
			bgbcToN[cfg] = []Node{n}
		} else {
			tmpNodeList = append(tmpNodeList, n)
			bgbcToN[cfg] = tmpNodeList
		}
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("error parsing query results: %w", err)
		return bgbcToN, err
	}

	return bgbcToN, err
}

// addBootConfigByGroup adds one or more BootConfig/BootGroup to the boot data database, assuming
// that the list of names are names for node groups, if it doesn't already exist. If an error occurs
// during any of the SQL queries, it is returned.
func (bddb BootDataDatabase) addBootConfigByGroup(groupNames []string, kernelUri, initrdUri, cmdline string) (map[string]string, error) {
	results := make(map[string]string)

	if len(groupNames) == 0 {
		return results, fmt.Errorf("no group names specified to add")
	}

	// See if group name exists, if passed.
	var existingBgNames []string
	for _, ebn := range groupNames {
		existingBgNames = append(existingBgNames, fmt.Sprintf("BootGroup(%s)", ebn))
	}
	qstr := fmt.Sprintf(`SELECT * FROM boot_groups WHERE name IN %s;`, stringSliceToSql(existingBgNames))
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("unable to query boot database: %w", err)
		return results, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	bgMap := make(map[string]BootGroup)
	for rows.Next() {
		var bg BootGroup
		err = rows.Scan(&bg.Id, &bg.BootConfigId, &bg.Name, &bg.Description)
		if err != nil {
			err = fmt.Errorf("could not scan SQL result: %w", err)
			return results, err
		}
		bgMap[bg.Name] = bg
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse query results: %w", err)
		return results, err
	}
	// If not, we are done processing the list of names. Check matches, if any.
	if len(bgMap) > 0 {
		// Check if there are any matching and/or non-matching BootGroups.
		bgNames := []string{}
		for bgName, _ := range bgMap {
			bgNames = append(bgNames, bgName)
		}
		_, nonExistingBootGroups := getMatches(groupNames, bgNames)

		// We don't change the BootConfig of an existing BootGroup
		// since we are adding, not updating. Therefore, we only
		// create a new BootConfig for new BootGroups.
		//
		// Create BootConfig for any new BootGroups.
		var (
			bcList []BootConfig
			bgList []BootGroup
		)
		for _, bgName := range nonExistingBootGroups {
			// Create boot config for node group.
			var bc BootConfig
			bc, err = NewBootConfig(kernelUri, initrdUri, cmdline)
			if err != nil {
				err = fmt.Errorf("could not create BootConfig: %w", err)
				return results, err
			}

			// Add new BootConfig to list so it can be added to the boot_configs
			// table later.
			bcList = append(bcList, bc)

			// Configure BootGroup with new BootConfig.
			if tempBg, ok := bgMap[bgName]; ok {
				tempBg.BootConfigId = bc.Id
				bgMap[bgName] = tempBg
			}

			// Create boot group for node group.
			var bg BootGroup
			newBgName := fmt.Sprintf("BootGroup(%s)", bgName)
			bgDesc := fmt.Sprintf("Boot group with name=%q", bgName)
			bg = NewBootGroup(bc.Id, newBgName, bgDesc)

			// Add BootGroup/BootConfig IDs to results.
			results[bg.Id] = bc.Id
		}

		// Add new BootGroups to boot_groups table.
		if len(bgList) > 0 {
			err = bddb.addBootGroups(bgList)
			if err != nil {
				err = fmt.Errorf("failed to add boot groups: %w", err)
				return results, err
			}
		}

		// Add new BootConfigs to boot_configs table.
		if len(bcList) > 0 {
			err = bddb.addBootConfigs(bcList)
			if err != nil {
				err = fmt.Errorf("failed to add boot configs: %w", err)
				return results, err
			}
		}
	}

	// We don't create new boot groups in BSS (TODO?), so results
	// is empty if we don't find an existing boot group to configure.
	return results, err
}

// addBootConfigByNode adds one or more BootConfig/BootGroup and BootGroupAssignment to the boot
// data database based on a slice of Node items and boot configuration parameters. If a
// BootGroup/BootConfig that is not for a node group already exists that matches the past
// kernel/initrd/cmdline, then a new one is not added and any Node/BootGroupAssignment that is added
// points to the existing BootGroup. Otherwise, a new BootConfig/BootGroup is added, and the
// newly-created Node/BootGroupAssignment items will point to the new BootGroup. If an error with
// any of the SQL queries occurs, it is returned.
func (bddb BootDataDatabase) addBootConfigByNode(nodeList []Node, kernelUri, initrdUri, cmdline string) (map[string]string, error) {
	var err error
	result := make(map[string]string)

	// Check to see if a node (not group) BootGroup and BootConfig exist with this
	// configuration. We will only add a new per-node BootGroup/BootConfig if one
	// does not already exist.
	var (
		existingBgList []BootGroup
		existingBcList []BootConfig
		numResults     int
		bg             BootGroup
		bc             BootConfig
		bgaList        []BootGroupAssignment
	)
	if len(nodeList) == 0 {
		return result, fmt.Errorf("no nodes specified to add boot configurations for")
	}
	existingBgList, existingBcList, numResults, err = bddb.GetBootConfigsByItems(kernelUri, initrdUri, cmdline)
	if err != nil {
		err = fmt.Errorf("could not get boot configs by kernel/initrd URI or params: %w", err)
		return result, err
	}
	// Create boot group and boot config with these parameters so we can compare them
	// with results from the database to see if they already exist.
	bgName := fmt.Sprintf("BootGroup(kernel=%q,initrd=%q,params=%q)", kernelUri, initrdUri, cmdline)
	bgDesc := fmt.Sprintf("Boot group for nodes with kernel=%q initrd=%q params=%q", kernelUri, initrdUri, cmdline)
	bc, err = NewBootConfig(kernelUri, initrdUri, cmdline)
	if err != nil {
		err = fmt.Errorf("could not create BootConfig: %w", err)
		return result, err
	}
	bg = NewBootGroup(bc.Id, bgName, bgDesc)
	addBcAndBg := true
	for i := 0; i < numResults; i++ {
		if bgName == existingBgList[i].Name &&
			bgDesc == existingBgList[i].Description &&
			bc.KernelUri == existingBcList[i].KernelUri &&
			bc.InitrdUri == existingBcList[i].InitrdUri &&
			bc.Cmdline == existingBcList[i].Cmdline {

			// A BootConfig/BootGroup with this configuration exists.
			// We will not add new ones.
			bc = existingBcList[i]
			bg = existingBgList[i]
			addBcAndBg = false
			break
		}
	}

	// If an existing BootConfig/BootGroup exists for this kernel/initrd/cmdline,
	// set bg and bc to it and create BootGroupAssignments for these nodes,
	// assigning them to the existing config.
	for _, node := range nodeList {
		// Create BootGroupAssignment for node.
		var bga BootGroupAssignment
		bga, err = NewBootGroupAssignment(bg.Id, node.Id)
		if err != nil {
			err = fmt.Errorf("could not create BootGroupAssignment: %w", err)
			return result, err
		}
		bgaList = append(bgaList, bga)
	}

	// Only add BootConfig/BootGroup if an existing one was not found based on
	// the kernel/initrd uri and params.
	if addBcAndBg {
		// Add new boot configs to boot_configs table.
		err = bddb.addBootConfigs([]BootConfig{bc})
		if err != nil {
			err = fmt.Errorf("could not add BootConfig %v: %w", bc, err)
			return result, err
		}

		// Add new boot groups to boot_groups table.
		err = bddb.addBootGroups([]BootGroup{bg})
		if err != nil {
			err = fmt.Errorf("could not add BootGroup %v: %w", bg, err)
			return result, err
		}

		// Add BootGroup/BootConfig to result.
		result[bg.Id] = bc.Id
	}

	// Add new nodes to nodes table.
	err = bddb.addNodes(nodeList)
	if err != nil {
		err = fmt.Errorf("failed to add nodes: %w", err)
		return result, err
	}

	// Add new boot group assignments to boot_group_assignments table.
	err = bddb.addBootGroupAssignments(bgaList)
	if err != nil {
		err = fmt.Errorf("could not add BootGroupAssignments %v: %w", bgaList, err)
		return result, err
	}

	return result, err
}

// deleteBootGroupsByName takes a slice of BootGroup names and deletes them from the boot_groups
// table of the database, returning a list of the boot groups that were deleted. If an error occurs
// with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootGroupsByName(names []string) (bgList []BootGroup, err error) {
	if len(names) == 0 {
		err = fmt.Errorf("no boot group names specified to delete")
		return bgList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_groups WHERE name IN %s RETURNING *;`, stringSliceToSql(names))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform boot group deletion in database: %w", err)
		return bgList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bg BootGroup
		err = rows.Scan(&bg.Id, &bg.BootConfigId, &bg.Name, &bg.Description)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into BootGroup: %w", err)
			return bgList, err
		}
		bgList = append(bgList, bg)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return bgList, err
	}

	return bgList, err
}

// deleteBootGroupsById takes a slice of BootGroup IDs and deletes the corresponding BootGroups from
// the boot_groups table of the database, returning a list of the boot groups that were deleted. If
// an error occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootGroupsById(bgIds []string) (bgList []BootGroup, err error) {
	if len(bgIds) == 0 {
		err = fmt.Errorf("no boot group IDs specified to delete")
		return bgList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_groups WHERE id IN %s RETURNING *;`, stringSliceToSql(bgIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform boot group deletion in database: %w", err)
		return bgList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bg BootGroup
		err = rows.Scan(&bg.Id, &bg.BootConfigId, &bg.Name, &bg.Description)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into BootGroup: %w", err)
			return bgList, err
		}
		bgList = append(bgList, bg)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return bgList, err
	}

	return bgList, err
}

// deleteBootConfigsById takes a slice of BootConfig IDs and deletes them from the boot_configs
// table of the database, returning a list of the boot configs that were deleted. If an error occurs
// with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootConfigsById(bcIds []string) (bcList []BootConfig, err error) {
	if len(bcIds) == 0 {
		err = fmt.Errorf("no boot config IDs specified to delete")
		return bcList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_configs WHERE id in %s RETURNING *;`, stringSliceToSql(bcIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform boot config deletion in database: %w", err)
		return bcList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bc BootConfig
		err = rows.Scan(&bc.Id, &bc.KernelUri, &bc.InitrdUri, &bc.Cmdline)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into BootConfig: %w", err)
			return bcList, err
		}
		bcList = append(bcList, bc)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return bcList, err
	}

	return bcList, err
}

// deleteBootConfigsByItems deletes boot configs with matching kernel URI, initrd URI, and params
// and deletes the nodes that are attached to them from the database. It returns a slice of deleted
// Nodes and a slice of deleted BootConfigs. If an error occurs with any of the queries, it is
// returned.
func (bddb BootDataDatabase) deleteBootConfigsByItems(kernelUri, initrdUri, cmdline string) ([]Node, []BootConfig, error) {
	var (
		bcList   []BootConfig
		nodeList []Node
	)

	qstr := `DELETE FROM boot_configs WHERE`
	strs := []string{kernelUri, initrdUri, cmdline}
	for first, i := true, 0; i < len(strs); i++ {
		if strs[i] != "" {
			if !first {
				qstr += ` AND`
			}
			switch i {
			case 0:
				qstr += fmt.Sprintf(` kernel_uri='%s'`, kernelUri)
			case 1:
				qstr += fmt.Sprintf(` initrd_uri='%s'`, initrdUri)
			case 2:
				qstr += fmt.Sprintf(` cmdline='%s'`, cmdline)
			}
			first = false
		}
	}
	qstr += ` RETURNING *;`
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform boot config deletion in database: %w", err)
		return nodeList, bcList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bc BootConfig
		err = rows.Scan(&bc.Id, &bc.KernelUri, &bc.InitrdUri, &bc.Cmdline)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into BootConfig: %w", err)
			return nodeList, bcList, err
		}
		bcList = append(bcList, bc)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return nodeList, bcList, err
	}
	rows.Close()

	var bcIdList []string
	for _, bc := range bcList {
		bcIdList = append(bcIdList, bc.Id)
	}
	qstr = fmt.Sprintf(`DELETE FROM boot_groups WHERE boot_config_id IN %s`, stringSliceToSql(bcIdList)) +
		` RETURNING *;`
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform boot group deletion: %w", err)
		return nodeList, bcList, err
	}
	var bgIdList []string
	for rows.Next() {
		var bg BootGroup
		err = rows.Scan(&bg.Id, &bg.BootConfigId, &bg.Name, &bg.Description)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into BootConfig: %w", err)
			return nodeList, bcList, err
		}
		bgIdList = append(bgIdList, bg.Id)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return nodeList, bcList, err
	}
	rows.Close()

	qstr = fmt.Sprintf(`DELETE FROM boot_group_assignments WHERE boot_group_id IN %s`, stringSliceToSql(bgIdList)) +
		` RETURNING *;`
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform boot group assignment deletion: %w", err)
		return nodeList, bcList, err
	}
	var nodeIdList []string
	for rows.Next() {
		var bga BootGroupAssignment
		err = rows.Scan(&bga.BootGroupId, &bga.NodeId)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into BootGroupAssignment: %w", err)
			return nodeList, bcList, err
		}
		nodeIdList = append(nodeIdList, bga.NodeId)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return nodeList, bcList, err
	}
	rows.Close()

	qstr = fmt.Sprintf(`DELETE FROM nodes WHERE id IN %s`, stringSliceToSql(nodeIdList)) +
		` RETURNING *;`
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform node deletion: %w", err)
		return nodeList, bcList, err
	}
	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into Node: %w", err)
			return nodeList, bcList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return nodeList, bcList, err
	}

	return nodeList, bcList, err
}

// deleteBootGroupAssignmentsByGroupId takes a slice of BootGroup IDs and deletes
// BootGroupAssignments whose boot group ID matches, returning a list of boot group assignments that
// were deleted. If an error occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootGroupAssignmentsByGroupId(bgIds []string) (bgaList []BootGroupAssignment, err error) {
	if len(bgIds) == 0 {
		err = fmt.Errorf("no boot group IDs specified for deleting boot group assignments")
		return bgaList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_group_assignments WHERE boot_group_id IN %s RETURNING *;`, stringSliceToSql(bgIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform boot group assignment deletion in database: %w", err)
		return bgaList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bga BootGroupAssignment
		err = rows.Scan(&bga.BootGroupId, &bga.NodeId)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into BootGroupAssignment: %w", err)
			return bgaList, err
		}
		bgaList = append(bgaList, bga)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return bgaList, err
	}

	return bgaList, err
}

// deleteBootGroupAssignmentsByNodeId takes a slice of Node IDs and deletes BootGroupAssignments
// whose node ID matches, returning a list of boot group assignments that were deleted. If an error
// occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootGroupAssignmentsByNodeId(nodeIds []string) (bgaList []BootGroupAssignment, err error) {
	if len(nodeIds) == 0 {
		err = fmt.Errorf("no node IDs specified for deleting boot group assignments")
		return bgaList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_group_assignments WHERE node_id IN %s RETURNING *;`, stringSliceToSql(nodeIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform boot group assignment deletion in database: %w", err)
		return bgaList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bga BootGroupAssignment
		err = rows.Scan(&bga.BootGroupId, &bga.NodeId)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into BootGroupAssignment: %w", err)
			return bgaList, err
		}
		bgaList = append(bgaList, bga)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return bgaList, err
	}

	return bgaList, err
}

// deleteNodesById takes a slice of Node IDs and deletes the corresponding nodes in the database. If
// an error occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteNodesById(nodeIds []string) (nodeList []Node, err error) {
	if len(nodeIds) == 0 {
		err = fmt.Errorf("no node IDs specified for deletion")
		return nodeList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM nodes WHERE id IN %s RETURNING *;`, stringSliceToSql(nodeIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform node deletion in database: %w", err)
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into Node: %w", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return nodeList, err
	}

	return nodeList, err
}

// deleteNodesByItems takes three slices: one of XNames (hosts), one of MAC addresses, and one of
// NIDs. If any of these match a node in the database, that node is deleted. A slice of deleted
// nodes is returned. If an error occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteNodesByItems(hosts, macs []string, nids []int32) (nodeList []Node, err error) {
	if len(hosts) == 0 && len(macs) == 0 && len(nids) == 0 {
		err = fmt.Errorf("no hosts, MAC addresses, or NIDs specified to delete nodes")
		return nodeList, err
	}
	qstr := `DELETE FROM nodes WHERE`
	lengths := []int{len(hosts), len(macs), len(nids)}
	for first, i := true, 0; i < len(lengths); i++ {
		if lengths[i] > 0 {
			if !first {
				qstr += ` OR`
			}
			switch i {
			case 0:
				qstr += fmt.Sprintf(` xname IN %s`, stringSliceToSql(hosts))
			case 1:
				// Ignore case when matching MAC addresses.
				var macsLower []string
				for _, mac := range macs {
					macsLower = append(macsLower, strings.ToLower(mac))
				}
				qstr += fmt.Sprintf(` boot_mac IN %s`, stringSliceToSql(macsLower))
			case 2:
				qstr += fmt.Sprintf(` nid IN %s`, int32SliceToSql(nids))
			}
			first = false
		}
	}
	// "RETURNING *" is Postgres-specific.
	qstr += ` RETURNING *;`
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("could not perform node deletion in database: %w", err)
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("could not scan deletion results into Node: %w", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("could not parse deletion query results: %w", err)
		return nodeList, err
	}

	return nodeList, err
}

// deleteBootConfigByGroup deletes the boot configs for specified node groups. This includes the
// BootGroup/BootConfig corresponding with the node group name, as well as any
// Node/BootGroupAssignment items that pointed to the deleted BootGroup. If an error with any of the
// SQL queries occurs, it is returned.
func (bddb BootDataDatabase) deleteBootConfigByGroup(groupNames []string) (nodeList []Node, bcList []BootConfig, err error) {
	if len(groupNames) == 0 {
		return nodeList, bcList, fmt.Errorf("no group names specified for deletion")
	}

	// Delete matching boot groups, store deleted ones.
	bgList, err := bddb.deleteBootGroupsByName(groupNames)
	if err != nil {
		err = fmt.Errorf("error deleting BootGroup(s): %w", err)
		return nodeList, bcList, err
	}

	// Store IDs of deleted BootGroups and their matching BootConfigs so we can match the former
	// to BootGroupAssignments and so we can delete BootConfigs matching the latter.
	var (
		bgIdList []string
		bcIdList []string
	)
	for _, bg := range bgList {
		bgIdList = append(bgIdList, bg.Id)
		bcIdList = append(bcIdList, bg.BootConfigId)
	}

	// Delete boot configs whose IDs match those from the deleted boot groups, store deleted
	// ones.
	bcList, err = bddb.deleteBootConfigsById(bcIdList)
	if err != nil {
		err = fmt.Errorf("error deleting BootConfig(s): %w", err)
		return nodeList, bcList, err
	}

	// Delete boot group assignments whose boot group ID matches that of any of the boot groups
	// that were deleted.
	var bgaList []BootGroupAssignment
	bgaList, err = bddb.deleteBootGroupAssignmentsByGroupId(bgIdList)
	if err != nil {
		err = fmt.Errorf("error deleting BootGroupAssignment(s): %w", err)
		return nodeList, bcList, err
	}

	// Store IDs of nodes whose BootGroupAssignments were deleted so we can delete those nodes.
	var nodeIdList []string
	for _, bga := range bgaList {
		nodeIdList = append(nodeIdList, bga.NodeId)
	}

	// Delete nodes whose ID matches that of any of the BootGroupAssignments that were deleted.
	nodeList, err = bddb.deleteNodesById(nodeIdList)
	if err != nil {
		err = fmt.Errorf("error deleting Node(s): %w", err)
		return nodeList, bcList, err
	}

	return nodeList, bcList, err
}

// deleteNodesWithBootConfigs deletes Node/BootGroupAssignment items from the database based on any
// matching XName, MAC address, or NID. If, for any nodes that are deleted, that node's BootGroup no
// longer has any other BootGroupAssignments pointing to it, that BootGroup and its corresponding
// BootConfig are also deleted. A slice of deleted Node items and a slice of deleted BootConfig
// items are returned. If an error occurs with any of the SQL queries, an error is returned.
func (bddb BootDataDatabase) deleteNodesWithBootConfigs(hosts, macs []string, nids []int32) (nodeList []Node, bcList []BootConfig, err error) {
	// MAC address comparison is case-insensitive.
	nodeList, err = bddb.deleteNodesByItems(hosts, macs, nids)
	if err != nil {
		err = fmt.Errorf("error deleting Node(s): %w", err)
		return nodeList, bcList, err
	}

	// Get node IDs to match with boot group assignments we must delete.
	var nodeIdList []string
	for _, node := range nodeList {
		nodeIdList = append(nodeIdList, node.Id)
	}

	// Delete boot group assignments for matching node IDs.
	var bgaList []BootGroupAssignment
	bgaList, err = bddb.deleteBootGroupAssignmentsByNodeId(nodeIdList)
	if err != nil {
		err = fmt.Errorf("error deleting BootGroupAssignment(s): %w", err)
		return nodeList, bcList, err
	}
	bgIdMap := make(map[string]string)
	for _, bga := range bgaList {
		if _, ok := bgIdMap[bga.BootGroupId]; !ok {
			bgIdMap[bga.BootGroupId] = bga.BootGroupId
		}
	}

	// Delete boot groups that were attached to the deleted nodes, but only those that don't
	// have any undeleted nodes attached to them.
	var uniqueBgIdList []string
	for _, bgId := range bgIdMap {
		nl, err := bddb.GetNodesByBootGroupId(bgId)
		if err != nil {
			err = fmt.Errorf("could not get nodes by boot group ID: %w", err)
			return nodeList, bcList, err
		}
		if len(nl) == 0 {
			uniqueBgIdList = append(uniqueBgIdList, bgId)
		}
	}
	if len(uniqueBgIdList) > 0 {
		// If no other nodes depend on these BootGroups/BootConfigs, delete them.
		var bgList []BootGroup
		bgList, err = bddb.deleteBootGroupsById(uniqueBgIdList)
		if err != nil {
			err = fmt.Errorf("error deleting BootGroup(s): %w", err)
			return nodeList, bcList, err
		}

		// Get list of deleted boot group IDs so we can delete their corresponding boot configs.
		var bcIdList []string
		for _, bg := range bgList {
			bcIdList = append(bcIdList, bg.BootConfigId)
		}

		// Delete boot configs that were connected to the deleted boot groups.
		bcList, err = bddb.deleteBootConfigsById(bcIdList)
		if err != nil {
			err = fmt.Errorf("error deleting BootConfig(s): %w", err)
			return nodeList, bcList, err
		}
	}

	return nodeList, bcList, err
}

// Add takes a bssTypes.BootParams and adds nodes and their boot configuration to the database. If a
// node or its configuration already exists, it is ignored. If one or more nodes are specified and a
// configuration exists that does not belong to an existing node group, that config is used for
// that/those node(s). One or more nodes can be specified by _either_ their XNames, boot MAC
// addresses, or NIDs. One or more node group names can be specified instead of XNames, but this is
// currently not supported by Add.
func (bddb BootDataDatabase) Add(bp bssTypes.BootParams) (result map[string]string, err error) {
	var (
		groupNames []string
		xNames     []string
		nodesToAdd []Node
	)

	// Check nodes table for any nodes that having a matching XName, MAC, or NID.
	existingNodeList, err := bddb.GetNodesByItems(bp.Macs, bp.Hosts, bp.Nids)
	if err != nil {
		err = ErrPostgresAdd{Err: err}
		return result, err
	}

	// Since we are _adding_ nodes, we will skip over existing nodes. It is assumed that existing
	// nodes already have a BootGroup with a corresponding BootConfig and a BootGroupAssignment.
	// So, when we add a new node, we will create a BootConfig and BootGroup for that node (if one
	// that does not belong to a node group and that has the same configuration does not already
	// exist), as well as a BootGroupAssignment asigning that node to that BootGroup.
	switch {
	case len(bp.Hosts) > 0:
		// Check each host to see if it is an XName or a node group name.
		for _, name := range bp.Hosts {
			xnameRaw := xnames.FromString(name)
			if xnameRaw == nil {
				groupNames = append(groupNames, name)
			} else if _, ok := xnameRaw.(xnames.Node); !ok {
				groupNames = append(groupNames, name)
			} else {
				xNames = append(xNames, name)
			}
		}
		// The BSS API only supports adding a boot config for _either_ a node group or a set
		// of nodes. Thus, we do either or here.
		if len(groupNames) > 0 {
			// Group name(s) specified, add boot config by group.
			result, err = bddb.addBootConfigByGroup(groupNames, bp.Kernel, bp.Initrd, bp.Params)
			if err != nil {
				err = ErrPostgresAdd{Err: err}
			}
			return result, err
		} else if len(xNames) > 0 {
			// XName(s) specified, add boot config by node(s).

			// Check nodes table for any nodes that having a matching XName, MAC, or NID.
			existingNodeList, err := bddb.GetNodesByItems(bp.Macs, bp.Hosts, bp.Nids)
			if err != nil {
				err = ErrPostgresAdd{Err: err}
				return result, err
			}

			// Determine nodes we need to add (ones that don't already exist).
			//
			// Nodes can be specified by XName, NID, or MAC address, so we query the list of existing
			// nodes using all three.
			// Make map of existing nodes with Xname as the key.
			existingNodeMap := make(map[string]Node)
			for _, n := range existingNodeList {
				existingNodeMap[n.Xname] = n
			}

			// Store list of nodes to add.
			for _, xname := range bp.Hosts {
				if existingNodeMap[xname] == (Node{}) {
					nodesToAdd = append(nodesToAdd, NewNode("", xname, 0))
				}
			}
		}
	case len(bp.Macs) > 0:
		// Make map of existing nodes with MAC address as the key.
		existingNodeMap := make(map[string]Node)
		for _, n := range existingNodeList {
			existingNodeMap[n.BootMac] = n
		}

		// Store list of nodes to add.
		for _, mac := range bp.Macs {
			if _, ok := existingNodeMap[strings.ToLower(mac)]; !ok {
				// Store using lower case version of MAC address. When the MAC
				// address is compared for retrieval/deletion/update, it will be
				// converted into lower case before comparison, effectively ignoring
				// case. This will prevent duplicate MAC addresses due to case
				// difference.
				nodesToAdd = append(nodesToAdd, NewNode(mac, "", 0))
			}
		}
	case len(bp.Nids) > 0:
		// Make map of existing nodes with Nid as the key.
		existingNodeMap := make(map[int32]Node)
		for _, n := range existingNodeList {
			existingNodeMap[n.Nid] = n
		}

		// Store list of nodes to add.
		for _, nid := range bp.Nids {
			if existingNodeMap[nid] == (Node{}) {
				nodesToAdd = append(nodesToAdd, NewNode("", "", nid))
			}
		}
	}

	if len(nodesToAdd) == 0 {
		err = ErrPostgresAdd{Err: ErrPostgresDuplicate{}}
		return result, err
	}

	// Add any nonexisting nodes, plus their boot config as needed.
	result, err = bddb.addBootConfigByNode(nodesToAdd, bp.Kernel, bp.Initrd, bp.Params)
	if err != nil {
		err = ErrPostgresAdd{Err: err}
	}

	return result, err
}

// Delete removes one or more nodes (and the corresponding BootGroupAssignment(s)) from the
// database, as well as the corresponding BootGroup/BootConfig if no other node uses the same boot
// config. If kernel URI, initrd URI, and params are specified, Delete will also remove any boot
// config (and matching boot group) matching them. A list of node IDs and a map of boot group IDs to
// boot config IDs that were deleted are returned. If an error occurs with the deletion, it is
// returned.
func (bddb BootDataDatabase) Delete(bp bssTypes.BootParams) (nodesDeleted, bcsDeleted []string, err error) {
	var (
		delNodes []Node
		delBcs   []BootConfig
	)

	// Delete nodes/boot configs by specifying one or more nodes. Leave boot configs that are
	// attached to nodes that won't be deleted.
	switch {
	case len(bp.Hosts) > 0:
		var (
			groupNames []string
			xNames     []string
		)
		// Check each host to see if it is an XName or a node group name.
		for _, name := range bp.Hosts {
			xnameRaw := xnames.FromString(name)
			if xnameRaw == nil {
				groupNames = append(groupNames, name)
			} else if _, ok := xnameRaw.(xnames.Node); !ok {
				groupNames = append(groupNames, name)
			} else {
				xNames = append(xNames, name)
			}
		}
		// The BSS API only supports adding a boot config for _either_ a node group or a set
		// of nodes. Thus, we do either or here.
		if len(groupNames) > 0 {
			// Group name(s) specified, add boot config by group.
			delNodes, delBcs, err = bddb.deleteBootConfigByGroup(groupNames)
			if err != nil {
				err = ErrPostgresDelete{Err: err}
				return nodesDeleted, bcsDeleted, err
			}
		} else if len(xNames) > 0 {
			// XName(s) specified, delete node(s) and relative boot configs.
			delNodes, delBcs, err = bddb.deleteNodesWithBootConfigs(xNames, []string{}, []int32{})
			if err != nil {
				err = ErrPostgresDelete{Err: err}
				return nodesDeleted, bcsDeleted, err
			}
		}
	case len(bp.Macs) > 0:
		// This deletion function will ignore the case of the passed MAC addresses by first
		// converting them to lower case before comparison.
		delNodes, delBcs, err = bddb.deleteNodesWithBootConfigs([]string{}, bp.Macs, []int32{})
		if err != nil {
			err = ErrPostgresDelete{Err: err}
			return nodesDeleted, bcsDeleted, err
		}

		for _, node := range delNodes {
			nodesDeleted = append(nodesDeleted, node.Id)
		}
		for _, bc := range delBcs {
			bcsDeleted = append(bcsDeleted, bc.Id)
		}
	case len(bp.Nids) > 0:
		delNodes, delBcs, err = bddb.deleteNodesWithBootConfigs([]string{}, []string{}, bp.Nids)
		if err != nil {
			err = ErrPostgresDelete{Err: err}
			return nodesDeleted, bcsDeleted, err
		}

		for _, node := range delNodes {
			nodesDeleted = append(nodesDeleted, node.Id)
		}
		for _, bc := range delBcs {
			bcsDeleted = append(bcsDeleted, bc.Id)
		}
	// Delete nodes/boot configs by specifying the boot configuration.
	case bp.Kernel != "" || bp.Initrd != "" || bp.Params != "":
		delNodes, delBcs, err = bddb.deleteBootConfigsByItems(bp.Kernel, bp.Initrd, bp.Params)
		if err != nil {
			err = ErrPostgresDelete{Err: err}
			return nodesDeleted, bcsDeleted, err
		}
	}

	for _, node := range delNodes {
		nodesDeleted = append(nodesDeleted, node.Id)
	}
	for _, bc := range delBcs {
		bcsDeleted = append(bcsDeleted, bc.Id)
	}

	return nodesDeleted, bcsDeleted, err
}

// Update modifies the boot parameters (and, optionally, the kernel and/or initramfs URI) of one or
// more existing nodes, specified by node ID, XName, or MAC address. If any of the passed nodes does
// not exist in the database, the operation aborts and an error is returned. A slice of strings is
// returned containing the node IDs of nodes whose values were updated.
func (bddb BootDataDatabase) Update(bp bssTypes.BootParams) (nodesUpdated []string, err error) {
	// Make sure all macs/xnames/nids passed exist; err if any do not.
	var (
		missingMacs   []string
		missingXnames []string
		missingNids   []int32
	)
	_, missingMacs, missingXnames, missingNids, err = bddb.CheckNodeExistence(bp.Macs, bp.Hosts, bp.Nids)
	if err != nil {
		err = ErrPostgresUpdate{Err: err}
		return nodesUpdated, err
	} else if len(missingMacs) > 0 || len(missingXnames) > 0 || len(missingNids) > 0 {
		err = ErrPostgresUpdate{
			Err: ErrPostgresNotExists{
				Data: fmt.Sprintf("nodes table: macs=%v xnames=%v nids=%v", missingMacs, missingXnames, missingNids),
			},
		}
		return nodesUpdated, err
	}

	// Make sure the new content isn't blank.
	lenParams := len(bp.Params)
	lenKernUri := len(bp.Kernel)
	lenInitrdUri := len(bp.Initrd)
	if lenParams == 0 && lenKernUri == 0 && lenInitrdUri == 0 {
		err = ErrPostgresUpdate{Err: fmt.Errorf("must specify at least one of params, kernel, or initrd")}
		return nodesUpdated, err
	}

	// Get requested nodes with their corresponding boot group and boot config.
	//
	// This is to keep track of which nodes need updating without duplicates (hence the map).
	// The value doesn't really matter here, since this map is used to check node existence.
	var nToBgbc map[Node]bgbc
	nToBgbc, err = bddb.getNodesWithConfigs(bp.Macs, bp.Hosts, bp.Nids)
	if err != nil {
		err = ErrPostgresUpdate{Err: err}
		return nodesUpdated, err
	}

	// Put node IDs from map above into slice for next step.
	nodeIds := make([]string, len(nToBgbc))
	idx := 0
	for n := range nToBgbc {
		nodeIds[idx] = n.Id
		idx++
	}

	// Get boot groups and boot configs that need updating with their corresponding node list.
	//
	// This is to keep track of which boot configs/groups can be deleted (so the new
	// config/group can be created or set, if it already exists) and which cannot (i.e. other
	// nodes not being updated depend on it. Nodes in this map are compared to nodes in nToBgbc
	// above to determine ig a boot config/group can be deleted.
	var bgbcToN map[bgbc][]Node
	bgbcToN, err = bddb.getConfigsWithNodes(nodeIds)
	if err != nil {
		err = ErrPostgresUpdate{Err: err}
		return nodesUpdated, err
	}

	// Query for boot configs/groups that have a similar config to that passed.
	//
	// This is to make sure a duplicate boot config/group is not added. When the boot configs/groups
	// are created for the nodes later (since the passed config is only partial), we compare
	// them to configs in this map and do not add it to the database if it already exists.
	var (
		sBcs    []BootConfig
		sBgs    []BootGroup
		lenSBcs int
	)
	similarBcs := make(map[BootConfig]BootGroup)
	sBgs, sBcs, lenSBcs, err = bddb.GetBootConfigsByItems(bp.Kernel, bp.Initrd, bp.Params)
	if err != nil {
		err = ErrPostgresUpdate{Err: err}
		return nodesUpdated, err
	}
	for i := 0; i < lenSBcs; i++ {
		sBcNoId := sBcs[i]
		sBcNoId.Id = "" // Blank out ID so comparison depends only on the config.
		similarBcs[sBcNoId] = sBgs[i]
	}

	// Determine boot configs/groups that need to be created, deleted, or left alone.
	//
	// Here, we have bgbcToN, which stores the boot configs/groups that correspond with nodes
	// that were specified mapped to _all_ of the nodes that each boot config/group corresponds
	// with. We also have nToBgbc, which stores each node that was specified mapped to the boot
	// config/group it corresponds with. We compare data between the two to determine which boot
	// configs/groups we can delete (no more nodes depend on it) and which we cannot (other
	// nodes still depend on it). We then generate the new config based on the old data and the
	// newly-passed data and compare it to any existing groups/configs. If this config already
	// exists, the nodes are pointed to the existing configs. Else, the new group/config is
	// added to the database and the nodes are pointed to the newly-created group/config.

	var bgbcToDel []bgbc                        // List of old boot configs/groups that can be deleted.
	bgbcToAdd := make(map[bgbc][]Node)          // Map of new boot configs/groups to be added, with their nodes.
	existingBgbcToNode := make(map[bgbc][]Node) // Map of existing boot configs/groups with their nodes.

	// Iterate through the list of boot configs/groups that relate to the nodes that were
	// passed.
	for ncfg, nList := range bgbcToN {
		// Determine which of the old boot configs/groups can be deleted.
		//
		// We will delete any old boot configs/groups that don't have any additional nodes
		// depending on them.
		delBgbc := true
		for _, n := range nList {
			if _, ok := nToBgbc[n]; ok {
				delBgbc = false
				break
			}
		}
		if delBgbc {
			bgbcToDel = append(bgbcToDel, ncfg)
		}

		// Create the new boot config/group for these nodes.
		//
		// The way we are "updating" is by:
		//
		// 1. copying the old group/config to a new one,
		// 2. updating the copy,
		// 3. deleting the old group/config, and
		// 4. pointing the nodes to the new group/config.
		//
		// However, a new group/config is only added to the database if an identical one
		// (sans ID) exists. If that is the case, Steps 1-2 are deleted and Step 4 becomes
		// "pointing the nodes to the existing group/config".
		var newBgbc bgbc
		newKernel := ncfg.Bc.KernelUri
		newInitrd := ncfg.Bc.InitrdUri
		newParams := ncfg.Bc.Cmdline
		if bp.Kernel != "" {
			newKernel = bp.Kernel
		}
		if bp.Initrd != "" {
			newInitrd = bp.Initrd
		}
		if bp.Params != "" {
			newParams = bp.Params
		}
		newBgName := fmt.Sprintf("BootGroup(kernel=%q,initrd=%q,params=%q)", newKernel, newInitrd, newParams)
		newBgDesc := fmt.Sprintf("Boot group for nodes with kernel=%q initrd=%q params=%q", newKernel, newInitrd, newParams)
		newBgbc.Bc, err = NewBootConfig(newKernel, newInitrd, newParams)
		if err != nil {
			err = ErrPostgresUpdate{Err: fmt.Errorf("could not create new BootConfig: %w", err)}
			return nodesUpdated, err
		}
		newBgbc.Bg = NewBootGroup(newBgbc.Bc.Id, newBgName, newBgDesc)

		// Check if an identical boot group/config already exists. If so, add the existing
		// config with its node list to existingBgbcToNode so we know not to create it
		// again. Otherwise, add it to bgbcToAdd with its node list so it will be created.
		newBcNoId := newBgbc.Bc
		newBcNoId.Id = "" // Blank out the ID so only the config will match.
		if tmpSimilarBg, ok := similarBcs[newBcNoId]; ok &&
			// If the config matches an existing one and the name/description matches the ones
			// we created (i.e. it is not a named group), then add it to the existing boot
			// group/config list.
			(tmpSimilarBg.Name == newBgName && tmpSimilarBg.Description == newBgDesc) {
			var sBgbc bgbc
			sBgbc.Bc = newBgbc.Bc
			sBgbc.Bg = tmpSimilarBg
			for _, n := range nList {
				// Only if the node is in the list of ones passed will it be
				// updated.
				if _, ok := nToBgbc[n]; ok {
					if tmpNodeList, ok := existingBgbcToNode[sBgbc]; !ok {
						existingBgbcToNode[sBgbc] = []Node{n}
					} else {
						tmpNodeList = append(tmpNodeList, n)
						existingBgbcToNode[sBgbc] = tmpNodeList
					}
				}
			}
		} else {
			// If the config doesn't match any existing ones or any existing ones are for a
			// named group, then add it to the list of groups/configs that will be created.
			for _, n := range nList {
				// Only if the node is in the list of ones passed will it be
				// updated.
				if _, ok := nToBgbc[n]; ok {
					if tmpNodeList, ok := bgbcToAdd[newBgbc]; !ok {
						bgbcToAdd[newBgbc] = []Node{n}
					} else {
						tmpNodeList = append(tmpNodeList, n)
						bgbcToAdd[newBgbc] = tmpNodeList
					}
				}
			}
		}
	}

	// Delete boot configs/groups marked for deletion.
	bcIds := []string{}
	for _, bgbc := range bgbcToDel {
		bcIds = append(bcIds, bgbc.Bc.Id)
	}
	execStr := `DELETE FROM boot_configs bc USING boot_groups bg` +
		` WHERE bg.boot_config_id=bc.id AND bc.id IN ` + stringSliceToSql(bcIds) +
		`;`
	execStr += ` DELETE FROM boot_groups bg WHERE bg.boot_config_id IN ` + stringSliceToSql(bcIds) + `;`
	_, err = bddb.DB.Exec(execStr)
	if err != nil {
		err = ErrPostgresUpdate{Err: fmt.Errorf("could not perform boot group/config deletion: %w", err)}
		return nodesUpdated, err
	}

	// Add boot configs/groups marked for addition.
	bcList := []BootConfig{}
	bgList := []BootGroup{}
	for bgbc := range bgbcToAdd {
		bcList = append(bcList, bgbc.Bc)
		bgList = append(bgList, bgbc.Bg)
	}
	err = bddb.addBootConfigs(bcList)
	if err != nil {
		err = ErrPostgresUpdate{Err: fmt.Errorf("could not add boot config(s): %w", err)}
		return nodesUpdated, err
	}
	err = bddb.addBootGroups(bgList)
	if err != nil {
		err = ErrPostgresUpdate{Err: fmt.Errorf("could not add boot config(s): %w", err)}
		return nodesUpdated, err
	}

	// Modify node boot group assignments to reflect update for new boot groups/configs.
	for bgbc, nodeList := range bgbcToAdd {
		nodeIds := make([]string, len(nodeList))
		for i := range nodeList {
			nodeIds[i] = nodeList[i].Id
		}
		err = bddb.updateNodeAssignment(nodeIds, bgbc.Bg.Id)
		if err != nil {
			err = ErrPostgresUpdate{Err: fmt.Errorf("could not update boot group assignments for nodes=%v: %w", nodeList, err)}
			return nodesUpdated, err
		}

		nodesUpdated = append(nodesUpdated, nodeIds...)
	}
	// Modify node boot group assignments to reflect update for existing boot groups/configs.
	for bgbc, nodeList := range existingBgbcToNode {
		nodeIds := make([]string, len(nodeList))
		for i := range nodeList {
			nodeIds[i] = nodeList[i].Id
		}
		err = bddb.updateNodeAssignment(nodeIds, bgbc.Bg.Id)
		if err != nil {
			err = ErrPostgresUpdate{Err: fmt.Errorf("could not update boot group assignments for nodes=%v: %w", nodeList, err)}
			return nodesUpdated, err
		}

		nodesUpdated = append(nodesUpdated, nodeIds...)
	}

	return nodesUpdated, err
}

// Set modifies existing boot parameters, kernel, or initramfs URIs and adds any
// new ones. Unlike Update, any nodes that do not already exist are added. Under
// the hood, Set determines which nodes do not exist and calls Add to add them
// with the new boot configuration, then determines which nodes do exist and
// calls Update to update them with the new boot configuration.
func (bddb BootDataDatabase) Set(bp bssTypes.BootParams) (err error) {
	// Make sure the new content isn't blank.
	lenParams := len(bp.Params)
	lenKernUri := len(bp.Kernel)
	lenInitrdUri := len(bp.Initrd)
	if lenParams == 0 && lenKernUri == 0 && lenInitrdUri == 0 {
		err = ErrPostgresUpdate{Err: fmt.Errorf("must specify at least one of params, kernel, or initrd")}
		return err
	}

	// Create BootParams struct for _new_ nodes that will be added
	addBp := bssTypes.BootParams{
		Kernel: bp.Kernel,
		Initrd: bp.Initrd,
		Params: bp.Params,
	}
	_, addBp.Macs, addBp.Hosts, addBp.Nids, err = bddb.CheckNodeExistence(bp.Macs, bp.Hosts, bp.Nids)
	if err != nil {
		err = ErrPostgresUpdate{Err: err}
		return err
	}

	// Create BootParams struct for _existing_ nodes that will be updated
	updateBp := bssTypes.BootParams{
		Kernel: bp.Kernel,
		Initrd: bp.Initrd,
		Params: bp.Params,
	}
	existingNodeList, err := bddb.GetNodesByItems(bp.Macs, bp.Hosts, bp.Nids)
	if err != nil {
		err = ErrPostgresAdd{Err: err}
		return err
	}
	for _, node := range existingNodeList {
		if node.BootMac != "" {
			updateBp.Macs = append(updateBp.Macs, node.BootMac)
		}
		if node.Xname != "" {
			updateBp.Hosts = append(updateBp.Hosts, node.Xname)
		}
		if node.Nid != 0 {
			updateBp.Nids = append(updateBp.Nids, node.Nid)
		}
	}

	// Add new nodes, boot config, boot group, and boot group assignments.
	//
	// The Add() function will take care of boot config/group deduplication.
	if len(addBp.Macs) > 0 || len(addBp.Hosts) > 0 || len(addBp.Nids) > 0 {
		_, err = bddb.Add(addBp)
		if err != nil {
			err = ErrPostgresSet{Err: fmt.Errorf("failed to add new boot configuration: %w", err)}
			return err
		}
	}

	// Update existing nodes, boot config, boot group, and boot group
	// assignments.
	//
	// The Update() function will take care of the deletion of dangling
	// configs.
	if len(existingNodeList) > 0 {
		_, err = bddb.Update(updateBp)
		if err != nil {
			err = ErrPostgresSet{Err: fmt.Errorf("failed to update existing boot configuration: %w", err)}
		}
	}

	return err
}

// GetBootParamsAll returns a slice of bssTypes.BootParams that contains all of the boot
// configurations for all nodes in the database. Each item contains node information (boot MAC
// address (if present), XName (if present), NID (if present)) as well as its associated boot
// configuration (kernel URI, initrd URI (if present), and parameters). If an error occurred while
// fetching the information, an error is returned.
func (bddb BootDataDatabase) GetBootParamsAll() ([]bssTypes.BootParams, error) {
	var results []bssTypes.BootParams

	qstr := "SELECT n.id, n.boot_mac, n.xname, n.nid, bga.boot_group_id, bc.id, bc.kernel_uri, bc.initrd_uri, bc.cmdline FROM nodes AS n" +
		" LEFT JOIN boot_group_assignments AS bga ON n.id=bga.node_id" +
		" JOIN boot_groups AS bg ON bga.boot_group_id=bg.id" +
		" JOIN boot_configs AS bc ON bg.boot_config_id=bc.id" +
		";"
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsAll: unable to query database: %w", err)}
		return results, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	bcToNode := make(map[BootConfig][]Node)
	for rows.Next() {
		var (
			node Node
			bc   BootConfig
			bgid string
		)
		err = rows.Scan(&node.Id, &node.BootMac, &node.Xname, &node.Nid,
			&bgid, &bc.Id, &bc.KernelUri, &bc.InitrdUri, &bc.Cmdline)
		if err != nil {
			err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsAll: could not scan SQL result: %w", err)}
			return results, err
		}

		// Add node to list corresponding to a BootConfig.
		if tempNodeList, ok := bcToNode[bc]; ok {
			tempNodeList = append(tempNodeList, node)
			bcToNode[bc] = tempNodeList
		} else {
			bcToNode[bc] = []Node{node}
		}
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsAll: could not parse query results: %w", err)}
		return results, err
	}
	// If not, we are done parsing the nodes and boot configs. Add to results.
	for bc, nodeList := range bcToNode {
		var bp bssTypes.BootParams
		bp.Kernel = bc.KernelUri
		bp.Initrd = bc.InitrdUri
		bp.Params = bc.Cmdline
		for _, node := range nodeList {
			if node.Xname != "" {
				bp.Hosts = append(bp.Hosts, node.Xname)
			}
			if node.BootMac != "" {
				bp.Macs = append(bp.Macs, node.BootMac)
			}
			if node.Nid != 0 {
				bp.Nids = append(bp.Nids, node.Nid)
			}
		}
		results = append(results, bp)
	}

	return results, err
}

// GetBootParamsByName returns a slice of bssTypes.BootParams that contains the boot configurations
// for nodes whose XNames are found in the passed slice of names. Each item contains node
// information (boot MAC address (if present), XName (if present), NID (if present)) as well as its
// associated boot configuration (kernel URI, initrd URI (if present), and parameters). If an error
// occurred while fetching the information, an error is returned.
func (bddb BootDataDatabase) GetBootParamsByName(names []string) ([]bssTypes.BootParams, error) {
	var results []bssTypes.BootParams

	// If input is empty, so is the output.
	if len(names) == 0 {
		return results, nil
	}

	qstr := "SELECT n.xname, bc.kernel_uri, bc.initrd_uri, bc.cmdline FROM nodes AS n" +
		" LEFT JOIN boot_group_assignments AS bga ON n.id=bga.node_id" +
		" JOIN boot_groups AS bg on bga.boot_group_id=bg.id" +
		" JOIN boot_configs AS bc ON bg.boot_config_id=bc.id" +
		" WHERE n.xname IN " + stringSliceToSql(names) +
		";"
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByName: unable to query database: %w", err)}
		return results, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var (
			name string
			bp   bssTypes.BootParams
		)
		err = rows.Scan(&name, &bp.Kernel, &bp.Initrd, &bp.Params)
		if err != nil {
			err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByName: could not scan SQL result: %w", err)}
			return results, err
		}
		bp.Hosts = append(bp.Hosts, name)

		results = append(results, bp)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByName: could not parse query results: %w", err)}
		return results, err
	}

	return results, err
}

// GetBootParamsByMac returns a slice of bssTypes.BootParams that contains the boot configurations
// for nodes whose boot MAC addresses are found in the passed slice of MAC addresses. Each item
// contains node information (boot MAC address (if present), XName (if present), NID (if present))
// as well as its associated boot configuration (kernel URI, initrd URI (if present), and
// parameters). If an error occurred while fetching the information, an error is returned.
func (bddb BootDataDatabase) GetBootParamsByMac(macs []string) ([]bssTypes.BootParams, error) {
	var results []bssTypes.BootParams

	// If input is empty, so is the output.
	if len(macs) == 0 {
		return results, nil
	}

	// Ignore case for MAC addresses.
	var macsLower []string
	for _, mac := range macs {
		macsLower = append(macsLower, strings.ToLower(mac))
	}

	qstr := "SELECT n.boot_mac, bc.kernel_uri, bc.initrd_uri, bc.cmdline FROM nodes AS n" +
		" LEFT JOIN boot_group_assignments AS bga ON n.id=bga.node_id" +
		" JOIN boot_groups AS bg on bga.boot_group_id=bg.id" +
		" JOIN boot_configs AS bc ON bg.boot_config_id=bc.id" +
		" WHERE n.boot_mac IN " + stringSliceToSql(macsLower) +
		";"
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByMac: unable to query database: %w", err)}
		return results, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var (
			mac string
			bp  bssTypes.BootParams
		)
		err = rows.Scan(&mac, &bp.Kernel, &bp.Initrd, &bp.Params)
		if err != nil {
			err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByMac: could not scan SQL result: %w", err)}
			return results, err
		}
		bp.Macs = append(bp.Macs, mac)

		results = append(results, bp)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByName: could not parse query results: %w", err)}
		return results, err
	}

	return results, err
}

// GetBootParamsByNid returns a slice of bssTypes.BootParams that contains the boot configurations
// for nodes whose NIDs are found in the passed slice of NIDs. Each item contains node information
// (boot MAC address (if present), XName (if present), NID (if present)) as well as its associated
// boot configuration (kernel URI, initrd URI (if present), and parameters). If an error occurred
// while fetching the information, an error is returned.
func (bddb BootDataDatabase) GetBootParamsByNid(nids []int32) ([]bssTypes.BootParams, error) {
	var results []bssTypes.BootParams

	// If input is empty, so is the output.
	if len(nids) == 0 {
		return results, nil
	}

	qstr := "SELECT n.nid, bc.kernel_uri, bc.initrd_uri, bc.cmdline FROM nodes AS n" +
		" LEFT JOIN boot_group_assignments AS bga ON n.id=bga.node_id" +
		" JOIN boot_groups AS bg on bga.boot_group_id=bg.id" +
		" JOIN boot_configs AS bc ON bg.boot_config_id=bc.id" +
		" WHERE n.nid IN " + int32SliceToSql(nids) +
		";"
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByNid: unable to query database: %w", err)}
		return results, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var (
			nid int32
			bp  bssTypes.BootParams
		)
		err = rows.Scan(&nid, &bp.Kernel, &bp.Initrd, &bp.Params)
		if err != nil {
			err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByNid: could not scan SQL result: %w", err)}
			return results, err
		}
		bp.Nids = append(bp.Nids, nid)

		results = append(results, bp)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = ErrPostgresGet{Err: fmt.Errorf("GetBootParamsByNid: could not parse query results: %w", err)}
		return results, err
	}

	return results, err
}
