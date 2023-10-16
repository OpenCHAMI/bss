package postgres

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/Cray-HPE/hms-bss/pkg/bssTypes"
	"github.com/docker/distribution/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/lib/pq"
)

const (
	dbName     = "bssdb"
	xNameRegex = `^x([0-9]{1,4})c([0-7])(s([0-9]{1,4}))?b([0])(n([0-9]{1,4}))?$`
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
		bddb    BootDataDatabase
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
		err = fmt.Errorf("postgres.GetNodesByBootGroupId: Unable to query database: %v", err)
		return nodeList, err
	}

	// rows.Next() returns false if either there is no next result (i.e. it
	// doesn't exist) or an error occurred. We return rows.Err() to
	// distinguish between the two cases.
	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("postgres.GetNodesByBootGroupId: Could not scan SQL result: %v", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("postgres.GetNodesByBootGroupId: Could not parse query results: %v", err)
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
		err = fmt.Errorf("postgres.GetBootConfigsAll: Unable to query database: %v", err)
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
			err = fmt.Errorf("postgres.GetBootConfigsAll: Could not scan SQL result: %v", err)
			return bgResults, bcResults, numResults, err
		}
		bg.BootConfigId = bc.Id

		bgResults = append(bgResults, bg)
		bcResults = append(bcResults, bc)
		numResults++
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("postgres.GetBootConfigsAll: Could not parse query results: %v", err)
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
		err = fmt.Errorf("postgres.GetBootConfigsAll: Unable to query database: %v", err)
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
			err = fmt.Errorf("postgres.GetBootConfigsAll: Could not scan SQL result: %v", err)
			return bgResults, bcResults, numResults, err
		}
		bg.BootConfigId = bc.Id

		bgResults = append(bgResults, bg)
		bcResults = append(bcResults, bc)
		numResults++
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("postgres.GetBootConfigsAll: Could not parse query results: %v", err)
		return bgResults, bcResults, numResults, err
	}

	return bgResults, bcResults, numResults, err
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

// Close calls the Close() method on the database object within the BootDataDatabase. If it errs, an
// error is returned.
func (bddb BootDataDatabase) Close() error {
	return bddb.DB.Close()
}

// CreateDB executes an SQL query to create the BSS database with the specified name (if it doesn't
// already exist) and creates the tables needed by BSS if they do not exist. If this query fails, an
// error is returned.
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

// addBootConfigByGroup adds one or more BootConfig/BootGroup to the boot data database, assuming
// that the list of names are names for node groups, if it doesn't already exist. If an error occurs
// during any of the SQL queries, it is returned.
func (bddb BootDataDatabase) addBootConfigByGroup(groupNames []string, kernelUri, initrdUri, cmdline string) (map[string]string, error) {
	results := make(map[string]string)

	if len(groupNames) == 0 {
		return results, fmt.Errorf("No group names specified to add.")
	}

	// See if group name exists, if passed.
	var existingBgNames []string
	for _, ebn := range groupNames {
		existingBgNames = append(existingBgNames, fmt.Sprintf("BootGroup(%s)", ebn))
	}
	qstr := fmt.Sprintf(`SELECT * FROM boot_groups WHERE name IN %s;`, stringSliceToSql(existingBgNames))
	rows, err := bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Unable to query boot database: %v", err)
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
			err = fmt.Errorf("Could not scan SQL result: %v", err)
			return results, err
		}
		bgMap[bg.Name] = bg
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse query results: %v", err)
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
				err = fmt.Errorf("Could not create BootConfig: %v", err)
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
				err = fmt.Errorf("postgres.Add: %v", err)
				return results, err
			}
		}

		// Add new BootConfigs to boot_configs table.
		if len(bcList) > 0 {
			err = bddb.addBootConfigs(bcList)
			if err != nil {
				err = fmt.Errorf("postgres.Add: %v", err)
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
		return result, fmt.Errorf("No nodes specified to add boot configurations for.")
	}
	existingBgList, existingBcList, numResults, err = bddb.GetBootConfigsByItems(kernelUri, initrdUri, cmdline)
	if err != nil {
		err = fmt.Errorf("Could not get boot configs by kernel/initrd URI or params: %v", err)
		return result, err
	}
	// Create boot group and boot config with these parameters so we can compare them
	// with results from the database to see if they already exist.
	bgName := fmt.Sprintf("BootGroup(kernel=%q,initrd=%q,params=%q)", kernelUri, initrdUri, cmdline)
	bgDesc := fmt.Sprintf("Boot group for nodes with kernel=%q initrd=%q params=%q", kernelUri, initrdUri, cmdline)
	bc, err = NewBootConfig(kernelUri, initrdUri, cmdline)
	if err != nil {
		err = fmt.Errorf("Could not create BootConfig: %v", err)
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
			err = fmt.Errorf("Could not create BootGroupAssignment: %v", err)
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
			err = fmt.Errorf("Could not add BootConfig %v: %v", bc, err)
			return result, err
		}

		// Add new boot groups to boot_groups table.
		err = bddb.addBootGroups([]BootGroup{bg})
		if err != nil {
			err = fmt.Errorf("Could not add BootGroup %v: %v", bg, err)
			return result, err
		}

		// Add BootGroup/BootConfig to result.
		result[bg.Id] = bc.Id
	}

	// Add new nodes to nodes table.
	err = bddb.addNodes(nodeList)
	if err != nil {
		err = fmt.Errorf("postgres.Add: %v", err)
		return result, err
	}

	// Add new boot group assignments to boot_group_assignments table.
	err = bddb.addBootGroupAssignments(bgaList)
	if err != nil {
		err = fmt.Errorf("Could not add BootGroupAssignments %v: %v", bgaList, err)
		return result, err
	}

	return result, err
}

// deleteBootGroupsByName takes a slice of BootGroup names and deletes them from the boot_groups
// table of the database, returning a list of the boot groups that were deleted. If an error occurs
// with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootGroupsByName(names []string) (bgList []BootGroup, err error) {
	if len(names) == 0 {
		err = fmt.Errorf("No boot group names specified to delete.")
		return bgList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_groups WHERE name IN %s RETURNING *;`, stringSliceToSql(names))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not perform boot group deletion in database: %v", err)
		return bgList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bg BootGroup
		err = rows.Scan(&bg.Id, &bg.BootConfigId, &bg.Name, &bg.Description)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into BootGroup: %v", err)
			return bgList, err
		}
		bgList = append(bgList, bg)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
		return bgList, err
	}

	return bgList, err
}

// deleteBootGroupsById takes a slice of BootGroup IDs and deletes the corresponding BootGroups from
// the boot_groups table of the database, returning a list of the boot groups that were deleted. If
// an error occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootGroupsById(bgIds []string) (bgList []BootGroup, err error) {
	if len(bgIds) == 0 {
		err = fmt.Errorf("No boot group IDs specified to delete.")
		return bgList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_groups WHERE id IN %s RETURNING *;`, stringSliceToSql(bgIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not perform boot group deletion in database: %v", err)
		return bgList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bg BootGroup
		err = rows.Scan(&bg.Id, &bg.BootConfigId, &bg.Name, &bg.Description)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into BootGroup: %v", err)
			return bgList, err
		}
		bgList = append(bgList, bg)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
		return bgList, err
	}

	return bgList, err
}

// deleteBootConfigsById takes a slice of BootConfig IDs and deletes them from the boot_configs
// table of the database, returning a list of the boot configs that were deleted. If an error occurs
// with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootConfigsById(bcIds []string) (bcList []BootConfig, err error) {
	if len(bcIds) == 0 {
		err = fmt.Errorf("No boot config IDs specified to delete.")
		return bcList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_configs WHERE id in %s RETURNING *;`, stringSliceToSql(bcIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not perform boot config deletion in database: %v", err)
		return bcList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bc BootConfig
		err = rows.Scan(&bc.Id, &bc.KernelUri, &bc.InitrdUri, &bc.Cmdline)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into BootConfig: %v", err)
			return bcList, err
		}
		bcList = append(bcList, bc)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
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
		err = fmt.Errorf("Could not perform boot config deletion in database: %v", err)
		return nodeList, bcList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bc BootConfig
		err = rows.Scan(&bc.Id, &bc.KernelUri, &bc.InitrdUri, &bc.Cmdline)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into BootConfig: %v", err)
			return nodeList, bcList, err
		}
		bcList = append(bcList, bc)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
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
		err = fmt.Errorf("Could not perform boot group deletion: %v", err)
		return nodeList, bcList, err
	}
	var bgIdList []string
	for rows.Next() {
		var bg BootGroup
		err = rows.Scan(&bg.Id, &bg.BootConfigId, &bg.Name, &bg.Description)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into BootConfig: %v", err)
			return nodeList, bcList, err
		}
		bgIdList = append(bgIdList, bg.Id)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
		return nodeList, bcList, err
	}
	rows.Close()

	qstr = fmt.Sprintf(`DELETE FROM boot_group_assignments WHERE boot_group_id IN %s`, stringSliceToSql(bgIdList)) +
		` RETURNING *;`
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not perform boot group assignment deletion: %v", err)
		return nodeList, bcList, err
	}
	var nodeIdList []string
	for rows.Next() {
		var bga BootGroupAssignment
		err = rows.Scan(&bga.BootGroupId, &bga.NodeId)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into BootGroupAssignment: %v", err)
			return nodeList, bcList, err
		}
		nodeIdList = append(nodeIdList, bga.NodeId)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
		return nodeList, bcList, err
	}
	rows.Close()

	qstr = fmt.Sprintf(`DELETE FROM nodes WHERE id IN %s`, stringSliceToSql(nodeIdList)) +
		` RETURNING *;`
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not perform node deletion: %v", err)
		return nodeList, bcList, err
	}
	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into Node: %v", err)
			return nodeList, bcList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
		return nodeList, bcList, err
	}

	return nodeList, bcList, err
}

// deleteBootGroupAssignmentsByGroupId takes a slice of BootGroup IDs and deletes
// BootGroupAssignments whose boot group ID matches, returning a list of boot group assignments that
// were deleted. If an error occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootGroupAssignmentsByGroupId(bgIds []string) (bgaList []BootGroupAssignment, err error) {
	if len(bgIds) == 0 {
		err = fmt.Errorf("No boot group IDs specified for deleting boot group assignments.")
		return bgaList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_group_assignments WHERE boot_group_id IN %s RETURNING *;`, stringSliceToSql(bgIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not perform boot group assignment deletion in database: %v", err)
		return bgaList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bga BootGroupAssignment
		err = rows.Scan(&bga.BootGroupId, &bga.NodeId)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into BootGroupAssignment: %v", err)
			return bgaList, err
		}
		bgaList = append(bgaList, bga)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
		return bgaList, err
	}

	return bgaList, err
}

// deleteBootGroupAssignmentsByNodeId takes a slice of Node IDs and deletes BootGroupAssignments
// whose node ID matches, returning a list of boot group assignments that were deleted. If an error
// occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteBootGroupAssignmentsByNodeId(nodeIds []string) (bgaList []BootGroupAssignment, err error) {
	if len(nodeIds) == 0 {
		err = fmt.Errorf("No node IDs specified for deleting boot group assignments.")
		return bgaList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM boot_group_assignments WHERE node_id IN %s RETURNING *;`, stringSliceToSql(nodeIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not perform boot group assignment deletion in database: %v", err)
		return bgaList, err
	}
	defer rows.Close()

	for rows.Next() {
		var bga BootGroupAssignment
		err = rows.Scan(&bga.BootGroupId, &bga.NodeId)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into BootGroupAssignment: %v", err)
			return bgaList, err
		}
		bgaList = append(bgaList, bga)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
		return bgaList, err
	}

	return bgaList, err
}

// deleteNodesById takes a slice of Node IDs and deletes the corresponding nodes in the database. If
// an error occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteNodesById(nodeIds []string) (nodeList []Node, err error) {
	if len(nodeIds) == 0 {
		err = fmt.Errorf("No node IDs specified for deletion.")
		return nodeList, err
	}
	// "RETURNING *" is Postgres-specific.
	qstr := fmt.Sprintf(`DELETE FROM nodes WHERE id IN %s RETURNING *;`, stringSliceToSql(nodeIds))
	var rows *sql.Rows
	rows, err = bddb.DB.Query(qstr)
	if err != nil {
		err = fmt.Errorf("Could not perform node deletion in database: %v", err)
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into Node: %v", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
		return nodeList, err
	}

	return nodeList, err
}

// deleteNodesByItems takes three slices: one of XNames (hosts), one of MAC addresses, and one of
// NIDs. If any of these match a node in the database, that node is deleted. A slice of deleted
// nodes is returned. If an error occurs with any of the SQL queries, it is returned.
func (bddb BootDataDatabase) deleteNodesByItems(hosts, macs []string, nids []int32) (nodeList []Node, err error) {
	if len(hosts) == 0 && len(macs) == 0 && len(nids) == 0 {
		err = fmt.Errorf("No hosts, MAC addresses, or NIDs specified to delete nodes.")
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
				qstr += fmt.Sprintf(` boot_mac IN %s`, stringSliceToSql(macs))
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
		err = fmt.Errorf("Could not perform node deletion in database: %v", err)
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		var n Node
		err = rows.Scan(&n.Id, &n.BootMac, &n.Xname, &n.Nid)
		if err != nil {
			err = fmt.Errorf("Could not scan deletion results into Node: %v", err)
			return nodeList, err
		}
		nodeList = append(nodeList, n)
	}
	// Did a rows.Next() return an error?
	if err = rows.Err(); err != nil {
		err = fmt.Errorf("Could not parse deletion query results: %v", err)
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
		return nodeList, bcList, fmt.Errorf("No group names specified for deletion.")
	}

	// Delete matching boot groups, store deleted ones.
	bgList, err := bddb.deleteBootGroupsByName(groupNames)
	if err != nil {
		err = fmt.Errorf("Error deleting BootGroup(s): %v", err)
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
		err = fmt.Errorf("Error deleting BootConfig(s): %v", err)
		return nodeList, bcList, err
	}

	// Delete boot group assignments whose boot group ID matches that of any of the boot groups
	// that were deleted.
	var bgaList []BootGroupAssignment
	bgaList, err = bddb.deleteBootGroupAssignmentsByGroupId(bgIdList)
	if err != nil {
		err = fmt.Errorf("Error deleting BootGroupAssignment(s): %v", err)
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
		err = fmt.Errorf("Error deleting Node(s): %v", err)
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
	nodeList, err = bddb.deleteNodesByItems(hosts, macs, nids)
	if err != nil {
		err = fmt.Errorf("Error deleting Node(s): %v", err)
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
		err = fmt.Errorf("Error deleting BootGroupAssignment(s): %v", err)
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
			err = fmt.Errorf("Could not get nodes by boot group ID: %v", err)
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
			err = fmt.Errorf("Error deleting BootGroup(s): %v", err)
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
			err = fmt.Errorf("Error deleting BootConfig(s): %v", err)
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
		reXName    = regexp.MustCompile(xNameRegex)
	)
	// Check each host to see if it is an XName or a node group name.
	for _, name := range bp.Hosts {
		match := reXName.FindString(name)
		if match == "" {
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
			err = fmt.Errorf("postgres.Add: %v", err)
			return result, err
		}
	} else if len(xNames) > 0 {
		// XName(s) specified, add boot config by node(s).

		// Check nodes table for any nodes that having a matching XName, MAC, or NID.
		existingNodeList, err := bddb.GetNodesByItems(bp.Macs, bp.Hosts, bp.Nids)
		if err != nil {
			err = fmt.Errorf("postgres.Add: %v", err)
			return result, err
		}

		// Since we are _adding_ nodes, we will skip over existing nodes. It is assumed that existing
		// nodes already have a BootGroup with a corresponding BootConfig and a BootGroupAssignment.
		// So, when we add a new node, we will create a BootConfig and BootGroup for that node (if one
		// that does not belong to a node group and that has the same configuration does not already
		// exist), as well as a BootGroupAssignment asigning that node to that BootGroup.

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

		// Add any nonexisting nodes, plus their boot config as needed.
		result, err = bddb.addBootConfigByNode(nodesToAdd, bp.Kernel, bp.Initrd, bp.Params)
		if err != nil {
			err = fmt.Errorf("postgres.Add: %v", err)
			return result, err
		}
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
	// Delete nodes/boot configs by specifying one or more nodes. Leave boot configs that are
	// attached to nodes that won't be deleted.
	switch {
	case len(bp.Hosts) > 0:
		var (
			groupNames []string
			xNames     []string
			reXName    = regexp.MustCompile(xNameRegex)
		)
		// Check each host to see if it is an XName or a node group name.
		for _, name := range bp.Hosts {
			match := reXName.FindString(name)
			if match == "" {
				groupNames = append(groupNames, name)
			} else {
				xNames = append(xNames, name)
			}
		}
		// The BSS API only supports adding a boot config for _either_ a node group or a set
		// of nodes. Thus, we do either or here.
		var (
			delNodes []Node
			delBcs   []BootConfig
		)
		if len(groupNames) > 0 {
			// Group name(s) specified, add boot config by group.
			delNodes, delBcs, err = bddb.deleteBootConfigByGroup(groupNames)
			if err != nil {
				err = fmt.Errorf("postgres.Add: %v", err)
				return nodesDeleted, bcsDeleted, err
			}
		} else if len(xNames) > 0 {
			// XName(s) specified, delete node(s) and relative boot configs.
			delNodes, delBcs, err = bddb.deleteNodesWithBootConfigs(xNames, bp.Macs, bp.Nids)
			if err != nil {
				err = fmt.Errorf("postgres.Delete: %v", err)
				return nodesDeleted, bcsDeleted, err
			}
		}

		for _, node := range delNodes {
			nodesDeleted = append(nodesDeleted, node.Id)
		}
		for _, bc := range delBcs {
			bcsDeleted = append(bcsDeleted, bc.Id)
		}
	case len(bp.Macs) > 0:
	case len(bp.Nids) > 0:
	}

	// Delete nodes/boot configs by specifying the boot configuration.
	if bp.Kernel != "" || bp.Initrd != "" || bp.Params != "" {
		var (
			delConfigNodes []Node
			delConfigs     []BootConfig
		)
		delConfigNodes, delConfigs, err = bddb.deleteBootConfigsByItems(bp.Kernel, bp.Initrd, bp.Params)
		if err != nil {
			err = fmt.Errorf("postgres.Delete: %v", err)
			return nodesDeleted, bcsDeleted, err
		}
		for _, node := range delConfigNodes {
			nodesDeleted = append(nodesDeleted, node.Id)
		}
		for _, bc := range delConfigs {
			bcsDeleted = append(bcsDeleted, bc.Id)
		}
	}

	return nodesDeleted, bcsDeleted, err
}

func (bddb BootDataDatabase) Update(bp bssTypes.BootParams) (err error) {
	return nil
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
			bc   BootConfig
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
			bp   bssTypes.BootParams
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

// GetBootParamsByMac returns a slice of bssTypes.BootParams that contains the boot configurations
// for nodes whose boot MAC addresses are found in the passed slice of MAC addresses. Each item
// contains node information (boot MAC address (if present), XName (if present), NID (if present))
// as well as its associated boot configuration (kernel URI, initrd URI (if present), and
// parameters). If an error occurred while fetching the information, an error is returned.
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
			bp  bssTypes.BootParams
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
			bp  bssTypes.BootParams
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
