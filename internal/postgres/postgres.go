package postgres

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Cray-HPE/hms-bss/pkg/bssTypes"
	"github.com/docker/distribution/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/lib/pq"
)

const dbName = "bssdb"

type Node struct{
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
	DB         *sqlx.DB
	// TODO: Utilize cache.
	//ImageCache map[string]Image
}

// makeKey creates a key from a key and subkey.  If key is not empty, it will
// be prepended with a '/' if it does not already start with one.  If subkey is
// not empty, it will be appended with a '/' if it does not already end with
// one.  The two will be concatenated with no '/' between them.
func makeKey(key, subkey string) string {
	ret := key
	if key != "" && key[0] != '/' {
		ret = "/" + key
	}
	if subkey != "" {
		if subkey[0] != '/' {
			ret += "/"
		}
		ret += subkey
	}
	return ret
}

// tagToColName extracts the field name from the JSON struct tag. Replace - with
// _.
// E.g: From `json:"params,omitempty"` comes `params`.
func tagToColName(tag string) string {
	re := regexp.MustCompile(`json:"([a-z0-9-]+)(?:,[a-z0-9-]+)*"`)
	colName := re.FindString(tag)
	return strings.Replace(colName, "-", "_", -1)
}

// fieldNameToColName converts the struct field name (in Pascal case) into
// the format for the database column (in snake case).
func fieldNameToColName(fieldName string) string {
	firstCap := regexp.MustCompile(`(.)([A-Z][a-z]+)`)
	allCaps := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	colName := firstCap.ReplaceAllString(fieldName, `${1}_${2}`)
	colName = allCaps.ReplaceAllString(colName, `${1}_${2}`)
	return strings.ToLower(colName)
}

// Connect opens a new connections to a Postgres database and ensures it is reachable.
// If not, an error is thrown.
func Connect(host string, port uint, user, password string, ssl bool) (BootDataDatabase, error) {
	var (
		sslmode string
		bddb BootDataDatabase
	)
	if ssl {
		sslmode = "verify-full"
	} else {
		sslmode = "disable"
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", user, password, host, port, dbName, sslmode)
	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		return bddb, err
	}
	// Create a new mapper which will use the struct field tag "json" instead of "db",
	// and ignore extra JSON config values, e.g. "omitempty".
	db.Mapper = reflectx.NewMapperTagFunc("json", fieldNameToColName, tagToColName)
	bddb.DB = db

	return bddb, err
}

// NewNode creates a new Node and populates it with the boot MAC address, XName, and NID specified.
// Before returning the Node, its ID is populated with a new unique identifier.
func NewNode(mac, xname string, nid int32) (n Node) {
	n.Id = uuid.Generate().String()
	n.BootMac = mac
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
		err = fmt.Errorf("Kernel URI cannot be blank")
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
		err = fmt.Errorf("Boot group ID or node MAC cannot be blank")
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
			err = fmt.Errorf("Error executing query to add node %v: %v", n, err)
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
			err = fmt.Errorf("Error executing query to add boot configs: %v", err)
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
			err = fmt.Errorf("Error executing query to add boot groups: %v", err)
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
			err = fmt.Errorf("Error executing query to add boot group assignments: %v", err)
			return err
		}
	}
	return err
}

// GetNodes returns a list of all nodes in the nodes table within bddb.
func (bddb BootDataDatabase) GetNodes() ([]Node, error) {
	nodeList := []Node{}
	qstr := `SELECT * FROM nodes;`
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not query node table in boot database: %v", err)
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("Could not scan results into Node: %v", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse query results: %v", err)
		return nodeList, err
	}

	return nodeList, err
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
				qstr += fmt.Sprintf(` boot_mac IN %s`, stringSliceToSql(macs))
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
		err = fmt.Errorf("Could not query node table in boot database: %v", err)
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("Could not scan results into Node: %v", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse query results: %v", err)
		return nodeList, err
	}

	return nodeList, err
}

func stringSliceToSql(ss []string) string {
	if len(ss) == 0 {
		return "('')"
	}
        sep := ""
        s := "("
        for i, st := range ss {
                s += sep + fmt.Sprintf("'%s'", st)
                if i == len(ss)-1 {
                        sep = ""
                } else {
                        sep = ", "
                }
        }
        s += ")"
        return s
}

func int32SliceToSql(is []int32) string {
	sep := ""
	s := "("
	for i, in := range is {
		s += sep + fmt.Sprintf("%d", in)
		if i == len(is)-1 {
			sep = ""
		} else {
			sep = ", "
		}
	}
	s += ")"
	return s
}

// Return the intersection of a and b (matches) and those elements in b but not in a (exclusions).
func getMatches(a, b []string) (matches, exclusions []string) {
	for _, aVal := range a {
		aInB := false
		for _, bVal := range b {
			if aVal == bVal {
				matches = append(matches, aVal)
				aInB = true
				break
			}
		}
		if !aInB {
			exclusions = append(exclusions, aVal)
		}
	}
	return matches, exclusions
}

func (bddb BootDataDatabase) Close() error {
	return bddb.DB.Close()
}

func (bddb BootDataDatabase) CreateDB(name string) (err error) {
	// Create the database.
	//
	// Since Postgres doesn't support IF NOT EXISTS for creating databases, we use
	// this workaround.
	// Source: https://stackoverflow.com/a/18389184
	execStr := "DO" +
	" $do$" +
	" BEGIN" +
	" 	IF EXISTS (SELECT FROM pg_database WHERE datname = '" + name + "') THEN" +
	" 		RAISE NOTICE 'Database already exists';  -- optional" +
	" 	ELSE" +
	" 		PERFORM dblink_exec('dbname=' || current_database(), create_database('" + name + "');" +
	" 	END IF;" +
	" END" +
	" $do$;"

	// Create the tables.
	execStr = `CREATE TABLE IF NOT EXISTS nodes (
		id varchar PRIMARY KEY,
		boot_mac varchar,
		xname varchar,
		nid int
	);
	CREATE TABLE IF NOT EXISTS boot_configs (
		id varchar PRIMARY KEY,
		kernel_uri varchar,
		initrd_uri varchar,
		cmdline varchar
	);
	CREATE TABLE IF NOT EXISTS boot_groups (
		id varchar PRIMARY KEY,
		boot_config_id varchar,
		name varchar,
		description varchar
	);
	CREATE TABLE IF NOT EXISTS boot_group_assignments (
		boot_group_id varchar,
		node_id varchar
	);`
	_, err = bddb.DB.Exec(execStr)
	if err != nil {
		err = fmt.Errorf("postgres.CreateDB: %v", err)
		return err
	}

	return err
}

func (bddb BootDataDatabase) addBootConfigByGroup(bp bssTypes.BootParams) (map[string]string, error) {
	result := make(map[string]string)

	// See if group name exists, if passed.
	qstr := fmt.Sprintf(`SELECT * FROM boot_groups WHERE name IN %s;`, stringSliceToSql(bp.Hosts))
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Unable to query boot database: %v", err)
		return result, err
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
			err = fmt.Errorf("Could not scan SQL result: %v", err)
			return result, err
		}
		bgMap[bg.Name] = bg
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse query results: %v", err)
		return result, err
	}
	// If not, we are done processing the list of names. Check matches, if any.
	if len(bgMap) > 0 {
		// Check if there are any matching and/or non-matching BootGroups.
		bgNames := []string{}
		for bgName, _ := range bgMap {
			bgNames = append(bgNames, bgName)
		}
		existingBootGroups, nonExistingBootGroups := getMatches(bp.Hosts, bgNames)

		// Add existing BootGroups to result.
		for _, bgName := range existingBootGroups {
			result[bgName] = bgMap[bgName].Id
		}

		// We don't change the BootConfig of an existing BootGroup
		// since we are adding, not updating. Therefore, we only
		// create BootConfigs for new BootGroups.
		//
		// Create BootConfigs for each new BootGroup.
		var bcList []BootConfig
		for _, bgName := range nonExistingBootGroups {
			var bc BootConfig
			bc, err = NewBootConfig(bp.Kernel, bp.Initrd, bp.Params)
			if err != nil {
				err = fmt.Errorf("postgres.Add: Could not create BootConfig: %v", err)
				return result, err
			}

			// Add new BootConfig to list so it can be added to the boot_configs
			// table later.
			bcList = append(bcList, bc)

			// Configure BootGroup with new BootConfig.
			if tempBg, ok := bgMap[bgName]; ok {
				tempBg.BootConfigId = bc.Id
				bgMap[bgName] = tempBg
			}

			// Add new BootGroup to result.
			result[bgName] = bgMap[bgName].Id
		}

		// Add new BootGroups to boot_groups table and result.
		var newBootGroups []BootGroup
		for _, bgName := range nonExistingBootGroups {
			// Add new BootGroup to list to be used in SQL query.
			newBootGroups = append(newBootGroups, bgMap[bgName])

			// Add new BootGroup to result.
			result[bgName] = bgMap[bgName].Id
		}
		err = bddb.addBootGroups(newBootGroups)
		if err != nil {
			err = fmt.Errorf("postgres.Add: %v", err)
			return result, err
		}

		// Add new BootConfigs.
		err = bddb.addBootConfigs(bcList)
		if err != nil {
			err = fmt.Errorf("postgres.Add: %v", err)
			return result, err
		}
	}

	// We don't create new boot groups in BSS (TODO?), so result
	// is empty if we don't find an existing boot group to configure.
	return result, err
}

func (bddb BootDataDatabase) addBootConfigByNode(bp bssTypes.BootParams) (map[string]string, error) {
	result := make(map[string]string)

	// Check nodes table for any nodes that having a matching XName, MAC, or NID.
	existingNodeList, err := bddb.GetNodesByItems(bp.Macs, bp.Hosts, bp.Nids)
	if err != nil {
		err = fmt.Errorf("postgres.Add: %v", err)
		return result, err
	}

	// Since we are adding nodes, we will skip over existing nodes. It is assumed that existing
	// nodes already have a BootGroup with a corresponding BootConfig and a BootGroupAssignment.
	// So, when we add a new node, we will create a BootConfig, a BootGroup for that node, and
	// a BootGroupAssignment asigning that node to that BootGroup.

	// Determine nodes we need to add (ones that don't already exist).
	//
	// Nodes can be specified by XName, NID, or MAC address, so we query the list of existing
	// nodes using all three.
	var nodesToAdd []Node
	switch {
	case len(bp.Macs) > 0:
		// Make map of existing nodes with MAC address as the key.
		existingNodeMap := make(map[string]Node)
		for _, n := range existingNodeList {
			existingNodeMap[n.BootMac] = n
		}

		// Store list of nodes to add.
		for _, mac := range bp.Macs {
			if existingNodeMap[mac] == (Node{}) {
				nodesToAdd = append(nodesToAdd, NewNode(mac, "", 0))
			}
		}
	case len(bp.Hosts) > 0:
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

	// Create BootConfig, BootGroup, and BootGroupAssignment for each node that we are
	// adding. Add each to their own list so they can submitted to the database via an
	// SQL query.
	var (
		bcList []BootConfig
		bgList []BootGroup
		bgaList []BootGroupAssignment
	)
	for _, node := range nodesToAdd {
		// Create BootConfig for node.
		var bc BootConfig
		bc, err = NewBootConfig(bp.Kernel, bp.Initrd, bp.Params)
		if err != nil {
			err = fmt.Errorf("postgres.Add: %v", err)
			return result, err
		}
		bcList = append(bcList, bc)

		// Create BootGroup for node.
		var bg BootGroup
		bgName := fmt.Sprintf("BootGroup%v", node)
		bgDesc := fmt.Sprintf("Boot group for node: %v", node)
		bg = NewBootGroup(bc.Id, bgName, bgDesc)
		bgList = append(bgList, bg)

		// Also add new BootGroup to result.
		result[bg.Name] = bg.Id

		// Create BootGroupAssignment for node.
		var bga BootGroupAssignment
		bga, err = NewBootGroupAssignment(bg.Id, node.Id)
		if err != nil {
			err = fmt.Errorf("postgres.Add: %v", err)
			return result, err
		}
		bgaList = append(bgaList, bga)
	}

	// Add new nodes to nodes table.
	err = bddb.addNodes(nodesToAdd)
	if err != nil {
		err = fmt.Errorf("postgres.Add: %v", err)
		return result, err
	}

	// Add new boot configs to boot_configs table.
	err = bddb.addBootConfigs(bcList)
	if err != nil {
		err = fmt.Errorf("postgres.Add: %v", err)
		return result, err
	}

	// Add new boot groups to boot_groups table.
	err = bddb.addBootGroups(bgList)
	if err != nil {
		err = fmt.Errorf("postgres.Add: %v", err)
		return result, err
	}

	// Add new boot group assignments to boot_group_assignments table.
	err = bddb.addBootGroupAssignments(bgaList)
	if err != nil {
		err = fmt.Errorf("postgres.Add: %v", err)
		return result, err
	}

	return result, err
}

func (bddb BootDataDatabase) Add(bp bssTypes.BootParams) (result map[string]string, err error) {
	// First, see if we are adding the config for a boot group that
	// already exists.
	result, err = bddb.addBootConfigByGroup(bp)
	if err != nil {
		err = fmt.Errorf("postgres.Add: %v", err)
		return result, err
	}
	// If boot config was added for a boot group, return the result.
	// Otherwise, we move on to add the config for node(s) if it/they
	// exist(s).
	if len(result) > 0 {
		return result, err
	}

	// If the no config was added for a boot group, then we try to
	// add a new node with the config, if it doesn't already exist.
	result, err = bddb.addBootConfigByNode(bp)
	if err != nil {
		err = fmt.Errorf("postgres.Add: %v", err)
		return result, err
	}
	return result, err
}

func (bddb BootDataDatabase) GetBootParamsAll() ([]bssTypes.BootParams, error) {
	var results []bssTypes.BootParams

	qstr := "SELECT n.id, n.boot_mac, n.xname, n.nid, bga.boot_group_id, bc.id, bc.kernel_uri, bc.initrd_uri, bc.cmdline FROM nodes AS n" +
		" LEFT JOIN boot_group_assignments AS bga ON n.id=bga.node_id" +
		" JOIN boot_groups AS bg ON bga.boot_group_id=bg.id" +
		" JOIN boot_configs AS bc ON bg.boot_config_id=bc.id" +
		";"
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("postgres.GetBootParamsAll: Unable to query database: %v", err)
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
			bc BootConfig
			bgid string
		)
		err = rows.Scan(&node.Id, &node.BootMac, &node.Xname, &node.Nid,
			&bgid, &bc.Id, &bc.KernelUri, &bc.InitrdUri, &bc.Cmdline)
		if err != nil {
			err = fmt.Errorf("postgres.GetBootParamsAll: Could not scan SQL result: %v", err)
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
		err = fmt.Errorf("postgres.GetBootParamsAll: Could not parse query results: %v", err)
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
		err = fmt.Errorf("postgres.GetBootParamsByName: Unable to query database: %v", err)
		return results, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var (
			name string
			bp bssTypes.BootParams
		)
		err = rows.Scan(&name, &bp.Kernel, &bp.Initrd, &bp.Params)
		if err != nil {
			err = fmt.Errorf("postgres.GetBootParamsByName: Could not scan SQL result: %v", err)
			return results, err
		}
		bp.Hosts = append(bp.Hosts, name)

		results = append(results, bp)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("postgres.GetBootParamsByName: Could not parse query results: %v", err)
		return results, err
	}

	return results, err
}

func (bddb BootDataDatabase) GetBootParamsByMac(macs []string) ([]bssTypes.BootParams, error) {
	var results []bssTypes.BootParams

	// If inout is empty, so is the output.
	if len(macs) == 0 {
		return results, nil
	}

	qstr := "SELECT n.boot_mac, bc.kernel_uri, bc.initrd_uri, bc.cmdline FROM nodes AS n" +
		" LEFT JOIN boot_group_assignments AS bga ON n.id=bga.node_id" +
		" JOIN boot_groups AS bg on bga.boot_group_id=bg.id" +
		" JOIN boot_configs AS bc ON bg.boot_config_id=bc.id" +
		" WHERE n.boot_mac IN " + stringSliceToSql(macs) +
		";"
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("postgres.GetBootParamsByMac: Unable to query database: %v", err)
		return results, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var (
			mac string
			bp bssTypes.BootParams
		)
		err = rows.Scan(&mac, &bp.Kernel, &bp.Initrd, &bp.Params)
		if err != nil {
			err = fmt.Errorf("postgres.GetBootParamsByMac: Could not scan SQL result: %v", err)
			return results, err
		}
		bp.Macs = append(bp.Macs, mac)

		results = append(results, bp)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("postgres.GetBootParamsByName: Could not parse query results: %v", err)
		return results, err
	}

	return results, err
}

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
		err = fmt.Errorf("postgres.GetBootParamsByNid: Unable to query database: %v", err)
		return results, err
	}
	defer rows.Close()

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var (
			nid int32
			bp bssTypes.BootParams
		)
		err = rows.Scan(&nid, &bp.Kernel, &bp.Initrd, &bp.Params)
		if err != nil {
			err = fmt.Errorf("postgres.GetBootParamsByNid: Could not scan SQL result: %v", err)
			return results, err
		}
		bp.Nids = append(bp.Nids, nid)

		results = append(results, bp)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("postgres.GetBootParamsByNid: Could not parse query results: %v", err)
		return results, err
	}

	return results, err
}
