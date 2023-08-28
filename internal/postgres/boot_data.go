package postgres

import (
	"fmt"
	"hash/fnv"

	"github.com/docker/distribution/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

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

func AddImage(db *sqlx.DB, image Image) error {
	db.ImageCache[makeImageKey(image.Type, image.Path)] = image
	_, err := db.Exec(`INSERT INTO images (image_id, path, params, type, format) VALUES ($1, $2, $3, $4, $5)`,
		image.ImageId, image.Path, image.Params, image.Type, image.Format)
	return err
}

func AddBootConfig(db *sqlx.DB, bootConfig BootConfig) error {
	_, err := db.Exec(`INSERT INTO boot_configs (boot_config_id, kernel_id, initrd_id, params) VALUES ($1, $2, $3, $4)`,
		bootConfig.BootConfigId, bootConfig.Kernel, bootConfig.Initrd, bootConfig.Params)
	return err
}

func AddBootGroup(db *sqlx.DB, bootGroup BootGroup) error {
	_, err := db.Exec(`INSERT INTO boot_groups (boot_group_id, boot_config_id, macs) VALUES ($1, $2, $3)`,
		bootGroup.BootGroupId, bootGroup.BootConfig.BootConfigId, bootGroup.Macs)
	return err
}
