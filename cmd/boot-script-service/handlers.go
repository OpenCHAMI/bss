package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	base "github.com/Cray-HPE/hms-base"
	"github.com/OpenCHAMI/bss/pkg/bssTypes"
)

// Common response handling
func sendJSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

func sendErrorResponse(w http.ResponseWriter, status int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	base.SendProblemDetailsGeneric(w, status, msg)
}

// Request parsing
type bootParamsRequest struct {
	params bssTypes.BootParams
	macs   []string
	hosts  []string
	nids   []int32
}

func parseBootParamsRequest(r *http.Request) (*bootParamsRequest, error) {
	req := &bootParamsRequest{}

	// Parse query parameters
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("failed to parse form: %v", err)
	}

	// Handle query parameters
	if mac := strings.Join(r.Form["mac"], ","); mac != "" {
		req.macs = strings.Split(mac, ",")
	}
	if host := strings.Join(r.Form["name"], ","); host != "" {
		req.hosts = strings.Split(host, ",")
	}
	if nid := strings.Join(r.Form["nid"], ","); nid != "" {
		for _, n := range strings.Split(nid, ",") {
			val, err := strconv.ParseInt(n, 0, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid NID format: %s", n)
			}
			req.nids = append(req.nids, int32(val))
		}
	}

	// Handle body if present
	if r.Body != nil && r.ContentLength > 0 {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %v", err)
		}

		if err := json.Unmarshal(body, &req.params); err != nil {
			return nil, fmt.Errorf("failed to parse request body: %v", err)
		}
	}

	// Merge query params into body params
	req.params.Macs = append(req.params.Macs, req.macs...)
	req.params.Hosts = append(req.params.Hosts, req.hosts...)
	req.params.Nids = append(req.params.Nids, req.nids...)

	return req, nil
}

// Handlers
func BootparametersGetAll(w http.ResponseWriter, r *http.Request) {
	var results []bssTypes.BootParams
	var err error

	if useSQL {
		results, err = bssdb.GetBootParamsAll()
		if err != nil {
			log.Printf("Failed to retrieve boot parameters from Postgres: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve boot parameters")
			return
		}
		debugf("Retrieved boot configs for %d nodes", len(results))
	} else {
		results = getBootParamsFromEtcd()
	}

	sendJSONResponse(w, http.StatusOK, results)
}

func BootparametersGet(w http.ResponseWriter, r *http.Request) {
	req, err := parseBootParamsRequest(r)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request: %v", err)
		return
	}

	// If no parameters specified, return all
	if len(req.params.Macs) == 0 && len(req.params.Hosts) == 0 &&
		len(req.params.Nids) == 0 && req.params.Kernel == "" && req.params.Initrd == "" {
		BootparametersGetAll(w, r)
		return
	}

	var results []bssTypes.BootParams
	if useSQL {
		results, err = SqlGetBootParams(req.params.Macs, req.params.Hosts, req.params.Nids)
		if err != nil {
			log.Printf("Failed to retrieve boot parameters: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve boot parameters")
			return
		}
	} else {
		results = getBootParamsFromEtcdByIdentifiers(req.params)
	}

	if len(results) == 0 {
		sendErrorResponse(w, http.StatusNotFound, "No boot parameters found for specified criteria")
		return
	}

	sendJSONResponse(w, http.StatusOK, results)
}

func BootparametersPost(w http.ResponseWriter, r *http.Request) {
	req, err := parseBootParamsRequest(r)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request: %v", err)
		return
	}

	// Validate request
	if err := validateBootParams(req.params); err != nil {
		LogBootParameters("/bootparameters POST FAILED: %s", req.params)
		sendErrorResponse(w, http.StatusBadRequest, "Validation failed: %v", err)
		return
	}

	// Store parameters
	err, token := StoreNew(req.params)
	if err != nil {
		LogBootParameters("/bootparameters POST FAILED: %s", req.params)
		sendErrorResponse(w, http.StatusBadRequest, "Failed to store boot parameters: %v", err)
		return
	}

	LogBootParameters("/bootparameters POST", req.params)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if token != "" {
		w.Header().Set("BSS-Referral-Token", token)
	}
	w.WriteHeader(http.StatusCreated)
}

func BootparametersPut(w http.ResponseWriter, r *http.Request) {
	req, err := parseBootParamsRequest(r)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request: %v", err)
		return
	}

	// Validate request
	if err := validateBootParams(req.params); err != nil {
		LogBootParameters("/bootparameters PUT FAILED: %s", req.params)
		sendErrorResponse(w, http.StatusBadRequest, "Validation failed: %v", err)
		return
	}

	// Store parameters
	err, token := Store(req.params)
	if err != nil {
		LogBootParameters("/bootparameters PUT FAILED: %s", req.params)
		if herr, ok := base.GetHMSError(err); ok && herr.GetProblem() != nil {
			base.SendProblemDetails(w, herr.GetProblem(), 0)
		} else {
			sendErrorResponse(w, http.StatusBadRequest, "Failed to store boot parameters")
		}
		return
	}

	LogBootParameters("/bootparameters PUT", req.params)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if token != "" {
		w.Header().Set("BSS-Referral-Token", token)
	}
	w.WriteHeader(http.StatusOK)
}

func BootparametersPatch(w http.ResponseWriter, r *http.Request) {
	req, err := parseBootParamsRequest(r)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request: %v", err)
		return
	}

	// Validate request
	if err := validateBootParams(req.params); err != nil {
		LogBootParameters("/bootparameters PATCH FAILED: %s", req.params)
		sendErrorResponse(w, http.StatusBadRequest, "Validation failed: %v", err)
		return
	}

	if err := Update(req.params); err != nil {
		LogBootParameters("/bootparameters PATCH FAILED: %s", req.params)
		sendErrorResponse(w, http.StatusNotFound, "Failed to update boot parameters: %v", err)
		return
	}

	LogBootParameters("/bootparameters PATCH", req.params)
	w.WriteHeader(http.StatusOK)
}

func BootparametersDelete(w http.ResponseWriter, r *http.Request) {
	req, err := parseBootParamsRequest(r)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request: %v", err)
		return
	}

	if err := Remove(req.params); err != nil {
		LogBootParameters("/bootparameters DELETE FAILED: %s", req.params)
		sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	LogBootParameters("/bootparameters DELETE", req.params)
	w.WriteHeader(http.StatusOK)
}

// Helper functions
func validateBootParams(params bssTypes.BootParams) error {
	if err := params.CheckMacs(); err != nil {
		return fmt.Errorf("invalid MAC address: %v", err)
	}
	if err := params.CheckXnames(); err != nil {
		return fmt.Errorf("invalid xname: %v", err)
	}
	return nil
}

func getBootParamsFromEtcd() []bssTypes.BootParams {
	var results []bssTypes.BootParams

	// Get kernel and initrd parameters
	for _, image := range GetKernelInfo() {
		results = append(results, bssTypes.BootParams{
			Params: image.Params,
			Kernel: image.Path,
		})
	}
	for _, image := range GetInitrdInfo() {
		results = append(results, bssTypes.BootParams{
			Params: image.Params,
			Initrd: image.Path,
		})
	}

	// Get node-specific parameters
	if kvl, err := getTags(); err == nil {
		for _, x := range kvl {
			name := extractParamName(x)
			var bds BootDataStore
			if err := json.Unmarshal([]byte(x.Value), &bds); err == nil {
				bd := bdConvert(bds)
				results = append(results, bssTypes.BootParams{
					Hosts:     []string{name},
					Params:    bd.Params,
					Kernel:    bd.Kernel.Path,
					Initrd:    bd.Initrd.Path,
					CloudInit: bd.CloudInit,
				})
			}
		}
	}

	return results
}

func getBootParamsFromEtcdByIdentifiers(params bssTypes.BootParams) []bssTypes.BootParams {
	var results []bssTypes.BootParams

	// Handle kernel/initrd specific queries
	if params.Kernel != "" || params.Initrd != "" {
		results = append(results, getBootParamsByImage(params)...)
	}

	// Handle identifier-based queries
	nameValues := GetNamesAndValues()
	for name, value := range nameValues {
		if bp := matchBootParams(name, value, params); bp != nil {
			results = append(results, *bp)
		}
	}

	return results
}

func getBootParamsByImage(params bssTypes.BootParams) []bssTypes.BootParams {
	var results []bssTypes.BootParams

	for _, image := range GetKernelInfo() {
		if image.Path == params.Kernel {
			results = append(results, bssTypes.BootParams{
				Params: image.Params,
				Kernel: image.Path,
			})
		}
	}
	for _, image := range GetInitrdInfo() {
		if image.Path == params.Initrd {
			results = append(results, bssTypes.BootParams{
				Params: image.Params,
				Initrd: image.Path,
			})
		}
	}

	return results
}

func matchBootParams(name, value string, params bssTypes.BootParams) *bssTypes.BootParams {
	smc := LookupComponentByName(name)

	bd, err := ToBootData(value, nil, nil)
	if err != nil {
		log.Printf("Failed to parse etcd value for %s: %v", name, err)
		return nil
	}

	// Check if this component matches any of the requested identifiers
	if !matchesIdentifiers(&smc, params) {
		return nil
	}

	return &bssTypes.BootParams{
		Hosts:     []string{name},
		Params:    bd.Params,
		Kernel:    bd.Kernel.Path,
		Initrd:    bd.Initrd.Path,
		CloudInit: bd.CloudInit,
	}
}

func matchesIdentifiers(smc *SMComponent, params bssTypes.BootParams) bool {
	// Check hostname matches
	for _, host := range params.Hosts {
		if host == smc.ID || host == smc.Fqdn {
			return true
		}
	}

	// Check MAC matches
	for _, reqMac := range params.Macs {
		for _, mac := range smc.Mac {
			if strings.EqualFold(reqMac, mac) {
				return true
			}
		}
	}

	// Check NID matches
	if nid, err := smc.NID.Int64(); err == nil {
		for _, reqNid := range params.Nids {
			if int64(reqNid) == nid {
				return true
			}
		}
	}

	return false
}
