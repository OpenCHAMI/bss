package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	base "github.com/Cray-HPE/hms-base"
	"github.com/OpenCHAMI/smd/v2/pkg/rf"
	"github.com/OpenCHAMI/smd/v2/pkg/sm"
)

// StateManager defines the interface for accessing state manager data
type StateManager interface {
	// GetState retrieves the current state of components
	GetState() (*SMData, error)

	// GetComponentByName retrieves a component by its name
	GetComponentByName(name string) (SMComponent, bool)

	// GetComponentByMAC retrieves a component by its MAC address
	GetComponentByMAC(mac string) (SMComponent, bool)

	// GetComponentByNID retrieves a component by its NID
	GetComponentByNID(nid int) (SMComponent, bool)

	// RefreshState forces a refresh of the state data
	RefreshState() error
}

// SMComponent represents a component in the state manager
type SMComponent struct {
	base.Component
	Fqdn            string   `json:"FQDN"`
	Mac             []string `json:"MAC"`
	EndpointEnabled bool     `json:"EndpointEnabled"`
}

// SMData represents the complete state manager data
type SMData struct {
	Components []SMComponent                    `json:"Components"`
	IPAddrs    map[string]sm.CompEthInterfaceV2 `json:"IPAddresses"`
}

// HSMStateManager implements StateManager interface for Hardware State Manager
type HSMStateManager struct {
	client    *OAuthClient
	baseURL   string
	mutex     sync.Mutex
	state     *SMData
	stateMap  map[string]SMComponent
	timestamp int64
}

// FileStateManager implements StateManager interface for local file storage
type FileStateManager struct {
	filePath  string
	mutex     sync.Mutex
	state     *SMData
	stateMap  map[string]SMComponent
	timestamp int64
}

// NewHSMStateManager creates a new HSM state manager instance
func NewHSMStateManager(baseURL string, client *OAuthClient) *HSMStateManager {
	return &HSMStateManager{
		client:   client,
		baseURL:  baseURL + "/hsm/v2",
		stateMap: make(map[string]SMComponent),
	}
}

// NewFileStateManager creates a new file-based state manager instance
func NewFileStateManager(filePath string) *FileStateManager {
	return &FileStateManager{
		filePath: filePath,
		stateMap: make(map[string]SMComponent),
	}
}

// getMacs processes Ethernet NIC information to extract MAC addresses
func getMacs(comp *SMComponent, eth []*rf.EthernetNICInfo) {
	for _, e := range eth {
		if e.MACAddress == "" || strings.EqualFold(e.MACAddress, "not available") {
			continue
		}
		found := false
		for _, m := range comp.Mac {
			if m == e.MACAddress {
				found = true
				break
			}
		}
		if !found {
			comp.Mac = append(comp.Mac, e.MACAddress)
		}
	}
}

// ensureLegalMAC validates and formats a MAC address
func ensureLegalMAC(mac string) string {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		var macPieces []string
		currentPiece := ""
		for i, r := range mac {
			currentPiece = fmt.Sprintf("%s%c", currentPiece, r)
			if i%2 == 1 {
				macPieces = append(macPieces, currentPiece)
				currentPiece = ""
			}
		}

		mac = strings.Join(macPieces, ":")

		hw, err = net.ParseMAC(mac)
		if err != nil {
			return "not available"
		}
	}

	return hw.String()
}

// makeRequest is a helper function to make authenticated HTTP requests
func (h *HSMStateManager) makeRequest(endpoint string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, h.baseURL+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Add authentication if enabled
	authEnabled, err := TestSMAuthEnabled(authRetryCount, authRetryWait)
	if err != nil {
		return nil, fmt.Errorf("failed to test auth: %v", err)
	}

	if authEnabled {
		if err := h.client.JWTTestAndRefresh(); err != nil {
			return nil, fmt.Errorf("failed to refresh JWT: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	req.Close = true
	base.SetHTTPUserAgent(req, serviceName)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	return resp, nil
}

// fetchComponents retrieves and processes component data
func (h *HSMStateManager) fetchComponents() (*SMData, error) {
	resp, err := h.makeRequest("/State/Components?type=Node")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var smData SMData
	if err := json.NewDecoder(resp.Body).Decode(&smData); err != nil {
		return nil, fmt.Errorf("failed to decode components: %v", err)
	}
	return &smData, nil
}

// fetchEndpoints retrieves and processes component endpoints
func (h *HSMStateManager) fetchEndpoints(smData *SMData) error {
	resp, err := h.makeRequest("/Inventory/ComponentEndpoints?type=Node")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var ep sm.ComponentEndpointArray
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return fmt.Errorf("failed to decode endpoints: %v", err)
	}

	// Create index for faster lookups
	compsIndex := make(map[string]int, len(smData.Components))
	for i, c := range smData.Components {
		compsIndex[c.ID] = i
	}

	// Process endpoints
	for _, e := range ep.ComponentEndpoints {
		if cIndex, ok := compsIndex[e.ID]; ok {
			comp := &smData.Components[cIndex]
			comp.Fqdn = e.FQDN
			comp.EndpointEnabled = true

			// Add MAC address if valid
			if e.MACAddr != "" && !strings.EqualFold(e.MACAddr, "not available") &&
				!strings.EqualFold(e.MACAddr, "ff:ff:ff:ff:ff:ff") {
				comp.Mac = append(comp.Mac, e.MACAddr)
			}

			// Process NIC info based on endpoint type
			switch e.ComponentEndpointType {
			case sm.CompEPTypeSystem:
				getMacs(comp, e.RedfishSystemInfo.EthNICInfo)
			case sm.CompEPTypeManager:
				getMacs(comp, e.RedfishManagerInfo.EthNICInfo)
			}
		}
	}
	return nil
}

// fetchEthernetInterfaces retrieves and processes ethernet interface data
func (h *HSMStateManager) fetchEthernetInterfaces(smData *SMData) error {
	resp, err := h.makeRequest("/Inventory/EthernetInterfaces?type=Node")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var ethIfaces []sm.CompEthInterfaceV2
	if err := json.NewDecoder(resp.Body).Decode(&ethIfaces); err != nil {
		return fmt.Errorf("failed to decode ethernet interfaces: %v", err)
	}

	// Process ethernet interfaces
	addresses := make(map[string]sm.CompEthInterfaceV2)
	for _, e := range ethIfaces {
		// Add IP addresses
		for _, ip := range e.IPAddrs {
			if ip.IPAddr != "" {
				addresses[ip.IPAddr] = e
			}
		}

		// Add MAC addresses to components
		for i := range smData.Components {
			if smData.Components[i].ID == e.CompID {
				smData.Components[i].Mac = append(smData.Components[i].Mac, ensureLegalMAC(e.MACAddr))
			}
		}
	}

	smData.IPAddrs = addresses
	return nil
}

func (h *HSMStateManager) RefreshState() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Fetch and process all data
	smData, err := h.fetchComponents()
	if err != nil {
		return fmt.Errorf("failed to fetch components: %v", err)
	}

	if err := h.fetchEndpoints(smData); err != nil {
		return fmt.Errorf("failed to fetch endpoints: %v", err)
	}

	if err := h.fetchEthernetInterfaces(smData); err != nil {
		return fmt.Errorf("failed to fetch ethernet interfaces: %v", err)
	}

	// Update state
	h.state = smData
	h.stateMap = make(map[string]SMComponent)
	for _, comp := range smData.Components {
		h.stateMap[comp.ID] = comp
	}
	h.timestamp = time.Now().Unix()

	return nil
}

// HSMStateManager implementation
func (h *HSMStateManager) GetState() (*SMData, error) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.state == nil {
		if err := h.RefreshState(); err != nil {
			return nil, err
		}
	}
	return h.state, nil
}

func (h *HSMStateManager) GetComponentByName(name string) (SMComponent, bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.state == nil {
		if err := h.RefreshState(); err != nil {
			return SMComponent{}, false
		}
	}
	comp, ok := h.stateMap[name]
	return comp, ok
}

func (h *HSMStateManager) GetComponentByMAC(mac string) (SMComponent, bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.state == nil {
		if err := h.RefreshState(); err != nil {
			return SMComponent{}, false
		}
	}

	for _, comp := range h.state.Components {
		if !strings.EqualFold(comp.State, "empty") {
			for _, m := range comp.Mac {
				if strings.EqualFold(mac, m) {
					return comp, true
				}
			}
		}
	}
	return SMComponent{}, false
}

func (h *HSMStateManager) GetComponentByNID(nid int) (SMComponent, bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.state == nil {
		if err := h.RefreshState(); err != nil {
			return SMComponent{}, false
		}
	}

	for _, comp := range h.state.Components {
		if vnid, err := comp.NID.Int64(); err == nil && vnid == int64(nid) {
			return comp, true
		}
	}
	return SMComponent{}, false
}

// FileStateManager implementation
func (f *FileStateManager) GetState() (*SMData, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.state == nil {
		if err := f.RefreshState(); err != nil {
			return nil, err
		}
	}
	return f.state, nil
}

func (f *FileStateManager) GetComponentByName(name string) (SMComponent, bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.state == nil {
		if err := f.RefreshState(); err != nil {
			return SMComponent{}, false
		}
	}
	comp, ok := f.stateMap[name]
	return comp, ok
}

func (f *FileStateManager) GetComponentByMAC(mac string) (SMComponent, bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.state == nil {
		if err := f.RefreshState(); err != nil {
			return SMComponent{}, false
		}
	}

	for _, comp := range f.state.Components {
		if !strings.EqualFold(comp.State, "empty") {
			for _, m := range comp.Mac {
				if strings.EqualFold(mac, m) {
					return comp, true
				}
			}
		}
	}
	return SMComponent{}, false
}

func (f *FileStateManager) GetComponentByNID(nid int) (SMComponent, bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.state == nil {
		if err := f.RefreshState(); err != nil {
			return SMComponent{}, false
		}
	}

	for _, comp := range f.state.Components {
		if vnid, err := comp.NID.Int64(); err == nil && vnid == int64(nid) {
			return comp, true
		}
	}
	return SMComponent{}, false
}

func (f *FileStateManager) RefreshState() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	file, err := os.Open(f.filePath)
	if err != nil {
		return fmt.Errorf("failed to open state file: %v", err)
	}
	defer file.Close()

	var smData SMData
	if err := json.NewDecoder(file).Decode(&smData); err != nil {
		return fmt.Errorf("failed to decode state file: %v", err)
	}

	f.state = &smData
	f.stateMap = make(map[string]SMComponent)
	for _, comp := range smData.Components {
		f.stateMap[comp.ID] = comp
	}
	f.timestamp = time.Now().Unix()

	return nil
}
