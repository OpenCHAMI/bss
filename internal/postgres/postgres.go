package postgres

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"github.com/docker/distribution/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/lib/pq"
)

const dbName = "bssdb"

type Image struct {
	ImageId string `json:"image_id"`         // fnv hash of the path to the image
	Path    string `json:"path"`             // URL or path to the image
	Params  string `json:"params,omitempty"` // boot parameters associated with this image
	Type    string `json:"type"`             // type of image (kernel, initrd, etc.)
	Format  string `json:"format"`           // format of the image (vmlinuz, etc.)
}

type BootConfig struct {
	BootConfigId uuid.UUID `json:"boot_config_id"`
	Kernel       Image     `json:"kernel"`
	Initrd       Image     `json:"initrd"`
	Params       string    `json:"params,omitempty"` // boot parameters associated with this image
}

type BootGroup struct {
	BootGroupId uuid.UUID  `json:"boot_group_id"`
	BootConfig  BootConfig `json:"boot_config"`
	Macs        []string   `json:"macs"`
}

type BootDataDatabase struct {
	DB         *sqlx.DB
	ImageCache map[string]Image
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

// makeImageKey creates a key for an image.  It uses the path to the image to
// create a hash and then uses the type of the image and the hash to create a
// key.
func makeImageKey(imtype, path string) string {
	h := fnv.New64a()
	h.Write([]byte(path))
	return makeKey(imtype, fmt.Sprintf("%x", h.Sum(nil)))
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

// NewBootDB initializes a new BootDataDatabase and configures the tag/field
// mapping of the sqlx database object it contains.
func NewBootDB() (bddb BootDataDatabase) {
	// Create a new mapper which will use the struct field tag "json" instead of "db",
	// and ignore extra JSON config values, e.g. "omitempty".
	bddb.DB.Mapper = reflectx.NewMapperTagFunc("json", fieldNameToColName, tagToColName)
	return bddb
}

// Connect opens a new connections to a Postgres database and ensures it is reachable.
// If not, an error is thrown.
func Connect(host string, port uint, user, password string, ssl bool) (*sqlx.DB, error) {
	var sslmode string
	if ssl {
		sslmode = "verify-full"
	} else {
		sslmode = "disable"
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", user, password, host, port, dbName, sslmode)
	return sqlx.Connect("postgres", connStr)
}

func AddImage(bddb BootDataDatabase, image Image) error {
	bddb.ImageCache[makeImageKey(image.Type, image.Path)] = image
	_, err := bddb.DB.Exec(`INSERT INTO images (image_id, path, params, type, format) VALUES ($1, $2, $3, $4, $5)`,
		image.ImageId, image.Path, image.Params, image.Type, image.Format)
	return err
}

func AddBootConfig(bddb BootDataDatabase, bootConfig BootConfig) error {
	_, err := bddb.DB.Exec(`INSERT INTO boot_configs (boot_config_id, kernel_id, initrd_id, params) VALUES ($1, $2, $3, $4)`,
		bootConfig.BootConfigId, bootConfig.Kernel, bootConfig.Initrd, bootConfig.Params)
	return err
}

func AddBootGroup(bddb BootDataDatabase, bootGroup BootGroup) error {
	_, err := bddb.DB.Exec(`INSERT INTO boot_groups (boot_group_id, boot_config_id, macs) VALUES ($1, $2, $3)`,
		bootGroup.BootGroupId, bootGroup.BootConfig.BootConfigId, bootGroup.Macs)
	return err
}
