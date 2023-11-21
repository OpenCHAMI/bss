// MIT License
//
// (C) Copyright [2021-2022] Hewlett Packard Enterprise Development LP
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

//
// Shasta boot script server data store
//
// The initial implementation will simply store things in memory.
// Eventually this will go into a DB for persistent storage.
//

package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	base "github.com/Cray-HPE/hms-base"
	hmetcd "github.com/Cray-HPE/hms-hmetcd"
	"github.com/OpenCHAMI/bss/pkg/bssTypes"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/google/uuid"
)

const (
	kernelImageType   = "kernel"
	initrdImageType   = "initrd"
	keyMin            = " "
	keyMax            = "~"
	paramsPfx         = "/params/"
	endpointAccessPfx = "/endpoint-access"
)

type BootDataStore struct {
	Params        string             `json:"params,omitempty"`
	Kernel        string             `json:"kernel,omitempty"`        // Image storage key
	Initrd        string             `json:"initrd,omitempty"`        // Image storage key
	CloudInit     bssTypes.CloudInit `json:"cloud-init,omitempty"`    // Image storage key
	ReferralToken string             `json:"referral-token,omitempty` // UUID
}

type ImageData struct {
	Path   string `json:"path"`             // URL or path to the image
	Params string `json:"params,omitempty"` // boot parameters associated with this image
}

type BootData struct {
	Params        string
	Kernel        ImageData
	Initrd        ImageData
	CloudInit     bssTypes.CloudInit
	ReferralToken string
}

const DefaultTag = "Default"
const GlobalTag = "Global"

var dataStore map[string]BootDataStore = make(map[string]BootDataStore)
var imageCache = func() hmetcd.Kvi { s, _ := hmetcd.Open("mem:", ""); return s }()

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

func makeImageKey(imtype, path string) string {
	h := fnv.New64a()
	h.Write([]byte(path))
	return makeKey(imtype, fmt.Sprintf("%x", h.Sum(nil)))
}

func imageLookup(path, imtype string, kvl []hmetcd.Kvi_KV) (string, ImageData) {
	debugf("imageLookup('%s', %s,  %v)\n", path, imtype, kvl)
	for _, k := range kvl {
		var imdata ImageData
		err := json.Unmarshal([]byte(k.Value), &imdata)
		if err == nil {
			debugf("Unmarshal %s: %v", k.Value, imdata)
		} else {
			debugf("Unmarshal %s failed: %s", k.Value, err.Error())
		}
		if err == nil && imdata.Path == path {
			return k.Key, imdata
		}
	}
	return "", ImageData{}
}

func getImage(imtype, subkey string) (ImageData, error) {
	key := makeKey(imtype, subkey)
	val, exists, err := imageCache.Get(key)
	if !exists || err != nil {
		val, exists, err = kvstore.Get(key)
	}
	if err == nil && !exists {
		err = fmt.Errorf("Key '%s' does not exist", key)
	}
	var imdata ImageData
	if err == nil {
		err = json.Unmarshal([]byte(val), &imdata)
	}
	if err != nil {
		msg := fmt.Sprintf("Error looking up key %s: %s", key, err.Error())
		herr := base.NewHMSError("Storage", msg)
		herr.AddProblem(base.NewProblemDetailsStatus(msg, http.StatusInternalServerError))
		err = herr
	}
	return imdata, err
}

func getImageInfo(imtype string) []ImageData {
	var ret []ImageData
	kvl, err := getImages(imtype)
	if err == nil {
		for _, k := range kvl {
			var imdata ImageData
			if err = json.Unmarshal([]byte(k.Value), &imdata); err == nil {
				ret = append(ret, imdata)
			}
		}
	}
	return ret
}

func GetKernelInfo() []ImageData {
	return getImageInfo(kernelImageType)
}

func GetInitrdInfo() []ImageData {
	return getImageInfo(initrdImageType)
}

// Convert a data structure to json and store it at the given key
func storeData(key string, v interface{}) error {
	debugf("storeData(%s, %v)\n", key, v)
	data, err := json.Marshal(v)
	if err == nil {
		value := string(data)
		err = kvstore.Store(key, value)
		debugf("kvstore.Store(%s, %s) -> %v\n", key, value, err)
	}
	if err != nil {
		msg := fmt.Sprintf("Key %s storage of '%v' failed: %s\n", key, v, err.Error())
		herr := base.NewHMSError("Storage", msg)
		herr.AddProblem(base.NewProblemDetailsStatus(msg, http.StatusInternalServerError))
		err = herr
		debugf(msg)
	}
	return err
}

func unknownKeys() ([]hmetcd.Kvi_KV, error) {
	keyBase := paramsPfx + unknownPrefix
	return kvstore.GetRange(keyBase+keyMin, keyBase+keyMax)
}

func getImages(imtype string) ([]hmetcd.Kvi_KV, error) {
	return kvstore.GetRange(makeKey(imtype, keyMin), makeKey(imtype, keyMax))
}

func imageFind(path string, imtype string) string {
	kvMutex.Lock()
	defer kvMutex.Unlock()
	kvl, _ := getImages(imtype)
	ret, _ := imageLookup(path, imtype, kvl)
	return ret
}

var kvMutex sync.Mutex

func imageStore(path string, imtype string) string {
	debugf("ImageStore(%s, %s)\n", path, imtype)
	kvMutex.Lock()
	defer kvMutex.Unlock()
	kvstore.DistTimedLock(5)
	defer kvstore.DistUnlock()

	kvl, err := getImages(imtype)
	var k string
	var imdata ImageData
	if err == nil {
		k, imdata = imageLookup(path, imtype, kvl)
	}
	debugf("imageLookup() -> (%s, %v)\n", k, imdata)
	if k != "" {
		// This path is already stored, return the key for it
		return k
	}
	key := makeImageKey(imtype, path)
	imdata = ImageData{path, ""}
	err = storeData(key, imdata)
	if err != nil {
		debugf("Cannot store %s path %s: %v\n", imtype, path, err)
		key = ""
	}
	return key
}

func nidName(nid int) string {
	return fmt.Sprintf("nid%d", nid)
}

func Remove(bp bssTypes.BootParams) error {
	debugf("Remove(): Ready to remove %v\n", bp)
	var err error
	if useSQL {
		var (
			nodesDeleted []string
			bcsDeleted   []string
		)
		nodesDeleted, bcsDeleted, err = bssdb.Delete(bp)
		if err != nil {
			return err
		}
		debugf("Node IDs deleted: %v", nodesDeleted)
		debugf("Boot Config IDs deleted: %v", bcsDeleted)
		return err
	}
	for _, h := range bp.Hosts {
		e := removeHost(h)
		if err == nil {
			err = e
		}
	}
	for _, m := range bp.Macs {
		comp, ok := FindSMCompByMAC(m)
		if ok {
			e := removeHost(comp.ID)
			if err == nil {
				err = e
			}
		}
	}
	for _, n := range bp.Nids {
		comp, ok := FindSMCompByNid(int(n))
		if ok {
			e := removeHost(comp.ID)
			if err == nil {
				err = e
			}
		} else {
			e := removeHost(nidName(int(n)))
			if err == nil {
				err = e
			}
		}
	}
	e := removeImage(bp.Kernel, kernelImageType)
	if err == nil {
		err = e
	}
	e = removeImage(bp.Initrd, initrdImageType)
	if err == nil {
		err = e
	}
	return err
}

func removeHost(h string) error {
	key := paramsPfx + h
	_, exists, err := kvstore.Get(key)
	if !exists {
		err = fmt.Errorf("Key %s does not exist", key)
	} else if err == nil {
		err = kvstore.Delete(key)
	}
	if err != nil {
		msg := fmt.Sprintf("Key %s deletion: %s", h, err.Error())
		herr := base.NewHMSError("Storage", msg)
		herr.AddProblem(base.NewProblemDetailsStatus(msg, http.StatusInternalServerError))
		return herr
	}
	return nil
}

func removeImage(path, imtype string) error {
	var err error
	if path != "" {
		kvl, _ := getImages(imtype)
		key, _ := imageLookup(path, imtype, kvl)
		if key != "" {
			// We found the image.  First, remove references from the dataStore
			err = kvstore.Delete(key)
			_ = imageCache.Delete(key)
			if err != nil {
				msg := fmt.Sprintf("Key %s deletion: %v\n", key, err)
				herr := base.NewHMSError("Storage", msg)
				herr.AddProblem(base.NewProblemDetailsStatus(msg, http.StatusInternalServerError))
				return herr
			}
			// Now remove any references to this image
			kvl, err = getTags()
			if err == nil {
				for _, x := range kvl {
					var bds BootDataStore
					e := json.Unmarshal([]byte(x.Value), &bds)
					if e == nil {
						if imtype == kernelImageType && bds.Kernel == key {
							bds.Kernel = ""
							err = storeData(x.Key, bds)
						} else if imtype == initrdImageType && bds.Initrd == key {
							bds.Initrd = ""
							err = storeData(x.Key, bds)
						}
					}
				}
			}
		}
	}
	return err
}

func extractParamName(x hmetcd.Kvi_KV) (ret string) {
	if strings.HasPrefix(x.Key, paramsPfx) {
		ret = strings.TrimPrefix(x.Key, paramsPfx)
	}
	return ret
}

func StoreNew(bp bssTypes.BootParams) (error, string) {
	// postgres.Add will handle duplicates, and it is called in New().
	// Therefore, if Postgres is enabled, simply call Store().
	if useSQL {
		return Store(bp)
	}

	item := ""
	// Go through the entire struct.  We must be storing to new hosts or this
	// request must fail.
	switch {
	case len(bp.Hosts) > 0:
		for _, h := range bp.Hosts {
			_, err := lookupHost(h)
			if err == nil {
				item = h
				break
			}
		}
	case len(bp.Macs) > 0:
		// Deal with MAC addresses
		for _, m := range bp.Macs {
			comp, ok := FindSMCompByMAC(m)
			if ok {
				if _, err := lookupHost(comp.ID); err == nil {
					item = m
					break
				}
			}
		}
	case len(bp.Nids) > 0:
		// Deal with Nids addresses
		for _, n := range bp.Nids {
			comp, ok := FindSMCompByNid(int(n))
			if ok {
				if _, err := lookupHost(comp.ID); err == nil {
					item = fmt.Sprintf("%d", n)
					break
				}
			}
		}
	case bp.Kernel != "":
		if imageFind(bp.Kernel, kernelImageType) != "" {
			item = bp.Kernel
		}
	case bp.Initrd != "":
		if imageFind(bp.Initrd, initrdImageType) != "" {
			item = bp.Initrd
		}
	}
	if item != "" {
		return fmt.Errorf("Already exists: %s", item), ""
	} else {
		return Store(bp)
	}
}

func Store(bp bssTypes.BootParams) (error, string) {
	debugf("Store(%v)\n", bp)

	if useSQL {
		debugf("postgres.Add(%v)\n", bp)
		result, err := bssdb.Add(bp)
		if err != nil {
			return err, ""
		}
		debugf("postgres.Add result: %v\n", result)
		return err, uuid.New().String()
	}

	var kernel_id, initrd_id string
	if bp.Kernel != "" {
		kernel_id = imageStore(bp.Kernel, kernelImageType)
		if kernel_id == "" {
			return fmt.Errorf("Cannot store image path %s", bp.Kernel), ""
		}
	}
	if bp.Initrd != "" {
		initrd_id = imageStore(bp.Initrd, initrdImageType)
		if initrd_id == "" {
			return fmt.Errorf("Cannot store image path %s", bp.Initrd), ""
		}
	}

	referralToken := uuid.New().String()
	bd := BootDataStore{bp.Params, kernel_id, initrd_id, bp.CloudInit, referralToken}
	var err error
	switch {
	case len(bp.Hosts) > 0:
		for _, h := range bp.Hosts {
			err = storeData(paramsPfx+h, bd)
			if err != nil {
				break
			}
		}
	case len(bp.Macs) > 0:
		// Deal with MAC addresses
		for _, m := range bp.Macs {
			comp, ok := FindSMCompByMAC(m)
			if ok {
				err = storeData(paramsPfx+comp.ID, bd)
				if err != nil {
					break
				}
			} else {
				// If the State Manager doesn't know about
				// it, store based on the MAC address.
				err = storeData(paramsPfx+m, bd)
				if err != nil {
					break
				}
			}
		}
	case len(bp.Nids) > 0:
		// Deal with Nids addresses
		for _, n := range bp.Nids {
			comp, ok := FindSMCompByNid(int(n))
			if ok {
				err = storeData(paramsPfx+comp.ID, bd)
				if err != nil {
					break
				}
			} else {
				// If the State Manager doesn't know about
				// it, store based on the NID.
				err = storeData(paramsPfx+nidName(int(n)), bd)
				if err != nil {
					break
				}
			}
		}
	case kernel_id != "":
		idata := ImageData{bp.Kernel, bp.Params}
		debugf("Ready to store data: %s, %v\n", kernel_id, idata)
		err = storeData(kernel_id, idata)
		referralToken = "" // referralToken was not needed
	case initrd_id != "":
		err = storeData(initrd_id, ImageData{bp.Initrd, bp.Params})
		referralToken = "" // referralToken was not needed
	default:
		herr := base.NewHMSError("Storage", "Nothing to Store")
		herr.AddProblem(base.NewProblemDetailsStatus("Nothing to Store", http.StatusBadRequest))
		referralToken = "" // referralToken was not needed
	}
	debugf("Store referralToken: %s\n", referralToken)
	return err, referralToken
}

// The update function will update entries but not NULL out existing entries.
func Update(bp bssTypes.BootParams) error {
	debugf("Update(%v)\n", bp)

	// Perform postgres.Update() and return if postgres is enabled.
	if useSQL {
		debugf("postgres.Update(%v)", bp)
		nodesUpdated, err := bssdb.Update(bp)
		if err != nil {
			return err
		}
		debugf("Node IDs updated: %v", nodesUpdated)
		return err
	}

	var kernel_id, initrd_id string
	var err error
	if bp.Kernel != "" {
		kernel_id = imageStore(bp.Kernel, kernelImageType)
	}
	if bp.Initrd != "" {
		initrd_id = imageStore(bp.Initrd, initrdImageType)
	}
	checkHost := func(hostMap *map[string]BootDataStore, h string) error {
		_, ok := (*hostMap)[h]
		if !ok {
			bd, err := lookupHost(h)
			if err != nil {
				return err
			}
			(*hostMap)[h] = bd
		}
		return nil
	}
	hostMap := make(map[string]BootDataStore)
	for _, h := range bp.Hosts {
		err = checkHost(&hostMap, h)
		if err != nil {
			return err
		}
	}
	for _, m := range bp.Macs {
		comp, ok := FindSMCompByMAC(m)
		if ok {
			// We've mapped the mac address to a host name,
			// let's see if this host name has boot data.
			err = checkHost(&hostMap, comp.ID)
			if err != nil {
				err = checkHost(&hostMap, m)
			}
			if err != nil {
				return err
			}
		}
	}
	for _, n := range bp.Nids {
		comp, ok := FindSMCompByNid(int(n))
		if ok {
			err = checkHost(&hostMap, comp.ID)
			if err != nil {
				err = checkHost(&hostMap, nidName(int(n)))
			}
			if err != nil {
				return err
			}
		}
	}

	switch {
	case len(hostMap) > 0:
		for h, bd := range hostMap {
			updated := false
			if bp.Params != "" && bp.Params != bd.Params {
				updated = true
				bd.Params = bp.Params
			}
			if bp.Kernel != "" && kernel_id != bd.Kernel {
				updated = true
				bd.Kernel = kernel_id
			}
			if bp.Initrd != "" && initrd_id != bd.Initrd {
				updated = true
				bd.Initrd = initrd_id
			}
			if updateCloudInit(&bd.CloudInit, bp.CloudInit) {
				updated = true
			}
			if updated {
				err = storeData(paramsPfx+h, bd)
			}
		}
	case kernel_id != "":
		// If no hosts were specified, then we should update the
		// parameters associated with the kernel image.
		idata := ImageData{bp.Kernel, bp.Params}
		debugf("Ready to store data: %s, %v\n", kernel_id, idata)
		err = storeData(kernel_id, idata)
	case initrd_id != "":
		err = storeData(initrd_id, ImageData{bp.Initrd, bp.Params})
	default:
		// No changes required so we are done.
		return nil
	}
	return err
}

func updateCloudData(existing *bssTypes.CloudDataType, merge bssTypes.CloudDataType, dataType string) bool {
	var err error
	changed := false
	defer func() {
		if err != nil {
			log.Printf("PATCH request for %s failed: %s", dataType, err)
			temp, err := json.Marshal(existing)
			if err == nil {
				log.Printf("    Existing: %s", temp)
			}
			temp, err = json.Marshal(merge)
			if err == nil {
				log.Printf("    Patch:    %s", temp)
			}
		}
	}()

	if merge != nil && len(merge) != 0 {
		if *existing == nil || len(*existing) == 0 {
			*existing = merge
			changed = merge != nil
		} else {
			// Need to convert to JSON for merge
			var e, m, patched []byte
			m, err = json.Marshal(merge)
			if err != nil {
				return changed
			}
			e, err = json.Marshal(existing)
			if err != nil {
				return changed
			}
			patched, err = jsonpatch.MergePatch(e, m)
			if err == nil {
				var temp bssTypes.CloudDataType
				changed = !jsonpatch.Equal(e, patched)
				err = json.Unmarshal(patched, &temp)
				if err == nil {
					*existing = temp
				}
			}
		}
	}
	return changed
}

func updateCloudInit(d *bssTypes.CloudInit, p bssTypes.CloudInit) bool {
	changed := updateCloudData(&d.MetaData, p.MetaData, "MetaData")
	changed = updateCloudData(&d.UserData, p.UserData, "UserData") || changed
	// If the new PhoneHome data has anything set, take the entire new object.
	if p.PhoneHome.PublicKeyDSA != "" || p.PhoneHome.PublicKeyRSA != "" ||
		p.PhoneHome.PublicKeyECDSA != "" || p.PhoneHome.PublicKeyED25519 != "" ||
		p.PhoneHome.InstanceID != "" || p.PhoneHome.Hostname != "" ||
		p.PhoneHome.FQDN != "" {
		if !reflect.DeepEqual(p.PhoneHome, d.PhoneHome) {
			d.PhoneHome = p.PhoneHome
			changed = true
		}
	}
	return changed
}

func updateEndpointAccessed(name string, accessType bssTypes.EndpointType) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	key := fmt.Sprintf("%s/%s/%s", endpointAccessPfx, name, accessType)
	if err := kvstore.Store(key, timestamp); err != nil {
		log.Printf("Failed to store last access timestamp %s to key %s: %s",
			timestamp, key, err)
	}
}

func searchKeyspace(prefix string) ([]hmetcd.Kvi_KV, error) {
	// No kidding, the way you search in etcd is to search for a range where the first part of the range is the actual
	// prefix and the second part of the range is that same prefix with the last character 1 unicode greater.
	// > If range_end is key plus one (e.g., "aa"+1 == "ab", "a\xff"+1 == "b"), then the range request gets all keys
	// > prefixed with key.
	// https://github.com/etcd-io/etcd/pull/7206/commits/7e31ddd32a4511c436b14e30ef43756ac782d080

	rangeStart := prefix
	rangePrefix := prefix[:len(prefix)-1]
	rangeLastNextChar := prefix[len(prefix)-1:][0] + 1
	rangeEnd := fmt.Sprintf("%s%c", rangePrefix, rangeLastNextChar)

	return kvstore.GetRange(rangeStart, rangeEnd)
}

func getAccessesForPrefix(prefix string) (accesses []bssTypes.EndpointAccess, err error) {
	kvs, searchErr := searchKeyspace(prefix)
	if searchErr != nil {
		err = fmt.Errorf("failed to search keyspace: %w", searchErr)
		return
	}

	for _, kv := range kvs {
		endpointParts := strings.Split(kv.Key, "/")
		endpoint := endpointParts[len(endpointParts)-1]
		name := endpointParts[len(endpointParts)-2]

		lastEpoch, err := strconv.ParseInt(kv.Value, 0, 64)
		if err != nil {
			err = fmt.Errorf("failed to convert timestamp to int: %w", err)
		}

		newAccess := bssTypes.EndpointAccess{
			Name:      name,
			Endpoint:  bssTypes.EndpointType(endpoint),
			LastEpoch: lastEpoch,
		}

		accesses = append(accesses, newAccess)
	}

	return
}

func SearchEndpointAccessed(name string, endpointType bssTypes.EndpointType) (accesses []bssTypes.EndpointAccess,
	err error) {
	if name == "" && endpointType == "" {
		return getAccessesForPrefix(fmt.Sprintf("%s/", endpointAccessPfx))
	} else if name != "" && endpointType == "" {
		return getAccessesForPrefix(fmt.Sprintf("%s/%s/", endpointAccessPfx, name))
	} else if name != "" && endpointType != "" {
		var epoch int64
		epoch, err = getEndpointAccessed(name, endpointType)
		if err != nil {
			return
		}
		// epoch == 0 means the given name and endpoint combo has never been accessed.
		// A long existing bug/feature of bss has been to return a value in this case with a LastEpoch value of zero.
		// The following preserves that behavior, but only if the endpoint type is valid.
		if epoch == 0 {
			hasValidType := false
			for _, t := range bssTypes.EndpointTypes {
				if strings.EqualFold(string(endpointType), string(t)) {
					hasValidType = true
				}
			}
			if !hasValidType {
				return
			}
		}

		access := bssTypes.EndpointAccess{
			Name:      name,
			Endpoint:  endpointType,
			LastEpoch: epoch,
		}
		accesses = append(accesses, access)

		return
	} else {
		err = fmt.Errorf("invalid search combination of name (%s) and endpoint (%s)", name, endpointType)
	}

	return
}

func getEndpointAccessed(name string, endpointType bssTypes.EndpointType) (int64, error) {
	key := fmt.Sprintf("%s/%s/%s", endpointAccessPfx, name, endpointType)
	timestampString, exists, err := kvstore.Get(key)

	if err != nil {
		return -1, fmt.Errorf("failed to retreive last access timestamp at key %s: %w", key, err)
	}

	if !exists {
		// Magic number, 0 meaning never accessed.
		return 0, nil
	}

	ts, err := strconv.ParseInt(timestampString, 0, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to convert timestamp to int: %w", err)
	}

	return ts, nil
}

func getTags() ([]hmetcd.Kvi_KV, error) {
	return kvstore.GetRange(paramsPfx+keyMin, paramsPfx+keyMax)
}

func GetNamesAndValues() map[string]string {
	kvl, err := getTags()
	m := make(map[string]string)
	if err == nil {
		for _, x := range kvl {
			name := extractParamName(x)
			m[name] = x.Value
		}
	}
	return m
}

func GetNames() (ret []string) {
	kvl, err := getTags()
	if err == nil {
		for _, x := range kvl {
			ret = append(ret, extractParamName(x))
		}
	}
	return ret
}

func LookupBootData(name string) (BootData, error) {
	var bd BootData
	bds, err := lookupHost(name)
	if err != nil {
		return bd, err
	}
	bd = bdConvert(bds)
	return bd, nil
}

func lookupHost(name string) (BootDataStore, error) {
	key := paramsPfx + name
	val, exists, err := kvstore.Get(key)
	var bds BootDataStore
	if !exists && err == nil {
		err = fmt.Errorf("Key %s does not exist", key)
	}
	if err == nil {
		err = json.Unmarshal([]byte(val), &bds)
	}
	if err != nil {
		msg := fmt.Sprintf("Error looking up %s: %v", name, err)
		herr := base.NewHMSError("Storage", msg)
		herr.AddProblem(base.NewProblemDetailsStatus(msg, http.StatusNotFound))
		err = herr
	}
	return bds, err
}

// Function lookup() will look up the boot parameter data from the KV store
// service.  If the given name does not have boot parameter data, it will
// then check an alternate name if a non-null one is provided.  If the alternate
// does not have boot parameter data as well, it will then check the provided
// role tag to see if it is non-null.  If it is also null, it will then check
// the default tag.  If boot parameter data is found, it will then convert from
// storage format to an external format.  This conversion process involves
// looking up the keys for the kernel and initrd images to their actual values,
// namely their paths and any associated parameters.
func lookup(name, altName, role, defaultTag string) BootData {
	bds, err := lookupHost(name)
	if err != nil && name != altName && altName != "" {
		bds, err = lookupHost(altName)
	}

	var tmpErr error
	if err != nil && role != "" {
		bds, tmpErr = lookupHost(role)
		if tmpErr == nil {
			err = nil
		}
	}
	if err != nil && defaultTag != "" {
		bds, tmpErr = lookupHost(defaultTag)
		if tmpErr != nil {
			debugf("Boot data for %s not available: %v\n", name, err)
		} else {
			err = nil
		}
	}

	var bd BootData
	if err == nil {
		bd = bdConvert(bds)
	}
	return bd
}

func bdConvertUsingImageCache(bds BootDataStore, kernelImages map[string]ImageData, initrdImages map[string]ImageData) (ret BootData) {
	ret.Params = bds.Params
	ret.CloudInit = bds.CloudInit
	if bds.Kernel != "" {
		if value, ok := kernelImages[bds.Kernel]; ok {
			ret.Kernel = value
		} else {
			imdata, err := getImage(bds.Kernel, "")
			if err == nil {
				ret.Kernel = imdata
				kernelImages[bds.Kernel] = imdata
			}
		}
	}
	if bds.Initrd != "" {
		if value, ok := initrdImages[bds.Initrd]; ok {
			ret.Initrd = value
		} else {
			imdata, err := getImage(bds.Initrd, "")
			if err == nil {
				ret.Initrd = imdata
				initrdImages[bds.Initrd] = imdata
			}
		}
	}
	return ret
}

func bdConvert(bds BootDataStore) (ret BootData) {
	ret.Params = bds.Params
	ret.CloudInit = bds.CloudInit
	ret.ReferralToken = bds.ReferralToken
	if bds.Kernel != "" {
		imdata, err := getImage(bds.Kernel, "")
		if err == nil {
			ret.Kernel = imdata
		}
	}
	if bds.Initrd != "" {
		imdata, err := getImage(bds.Initrd, "")
		if err == nil {
			ret.Initrd = imdata
		}
	}
	return ret
}

func LookupByRole(role string) (BootData, error) {
	var bd BootData
	bds, err := lookupHost(role)
	if err != nil {
		return bd, err
	}
	bd = bdConvert(bds)
	return bd, err
}

func LookupGlobalData() (BootData, error) {
	return LookupByRole(GlobalTag)
}

func LookupComponentByName(name string) SMComponent {
	comp, _ := FindSMCompByNameInCache(name)
	return comp
}

func ToBootData(value string, kernelImages map[string]ImageData, initrdImages map[string]ImageData) (BootData, error) {
	var bds BootDataStore
	err := json.Unmarshal([]byte(value), &bds)
	var bd BootData
	if err != nil {
		msg := fmt.Sprintf("Error parsing %s: %v", value, err)
		herr := base.NewHMSError("Storage", msg)
		herr.AddProblem(base.NewProblemDetailsStatus(msg, http.StatusNotFound))
		err = herr
	} else {
		bd = bdConvertUsingImageCache(bds, kernelImages, initrdImages)
	}
	return bd, err
}

func LookupByName(name string) (BootData, SMComponent) {
	comp_name := name
	comp, ok := FindSMCompByName(name)
	role := ""
	if ok {
		comp_name = comp.ID
		role = comp.Role
	}
	if useSQL {
		var result BootData
		bps, err := bssdb.GetBootParamsByName([]string{name})
		if err != nil {
			err = fmt.Errorf("Could not retrieve boot parameters with name %q: %v", name, err)
			log.Printf("ERROR: %v", err)
			return result, comp
		}
		if len(bps) == 0 {
			// Not found.
			log.Printf("WARNING: Name %q did not return any results.", name)
			return result, comp
		} else if len(bps) > 1 {
			debugf("BootParams returned: %v", bps)
			log.Printf("WARNING: More than 1 node found for name %q, taking first one: %v", name, bps[0])
		}
		result.Kernel = ImageData{bps[0].Kernel, ""}
		result.Initrd = ImageData{bps[0].Initrd, ""}
		result.Params = bps[0].Params
		return result, comp
	}
	return lookup(comp_name, name, role, DefaultTag), comp
}

func LookupByMAC(mac string) (BootData, SMComponent) {
	comp_name := mac
	comp, ok := FindSMCompByMAC(mac)
	role := ""
	if ok {
		comp_name = comp.ID
		role = comp.Role
	}
	if useSQL {
		var result BootData
		bps, err := bssdb.GetBootParamsByMac([]string{mac})
		if err != nil {
			err = fmt.Errorf("Could not retrieve boot parameters with mac %q: %v", mac, err)
			log.Printf("ERROR: %v", err)
			return result, comp
		}
		if len(bps) == 0 {
			// Not found.
			log.Printf("WARNING: MAC %q did not return any results.", mac)
			return result, comp
		} else if len(bps) > 1 {
			debugf("BootParams returned: %v", bps)
			log.Printf("WARNING: More than 1 node found for MAC %q, taking first one: %v", mac, bps[0])
		}
		result.Kernel = ImageData{bps[0].Kernel, ""}
		result.Initrd = ImageData{bps[0].Initrd, ""}
		result.Params = bps[0].Params
		return result, comp
	}
	return lookup(comp_name, mac, role, DefaultTag), comp
}

func LookupByNid(nid int) (BootData, SMComponent) {
	nid_str := nidName(nid)
	comp_name := nid_str
	comp, ok := FindSMCompByNid(nid)
	role := ""
	if ok {
		comp_name = comp.ID
		role = comp.Role
	}
	if useSQL {
		var result BootData
		bps, err := bssdb.GetBootParamsByNid([]int32{int32(nid)})
		if err != nil {
			err = fmt.Errorf("Could not retrieve boot parameters with NID %d: %v", nid, err)
			log.Printf("ERROR: %v", err)
			return result, comp
		}
		if len(bps) == 0 {
			// Not found.
			log.Printf("WARNING: NID %d did not return any results.", nid)
			return result, comp
		} else if len(bps) > 1 {
			debugf("BootParams returned: %v", bps)
			log.Printf("WARNING: More than 1 node found for NID %d, taking first one: %v", nid, bps[0])
		}
		result.Kernel = ImageData{bps[0].Kernel, ""}
		result.Initrd = ImageData{bps[0].Initrd, ""}
		result.Params = bps[0].Params
		return result, comp
	}
	return lookup(comp_name, nid_str, role, DefaultTag), comp
}

func dumpDataStore() {
	kvl, err := kvstore.GetRange(keyMin, keyMax)
	if err == nil {
		for _, x := range kvl {
			fmt.Printf("%s: %s\n", x.Key, x.Value)
		}
	}
}
