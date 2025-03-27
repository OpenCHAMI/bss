package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	base "github.com/Cray-HPE/hms-base"
	"github.com/OpenCHAMI/smd/v2/pkg/rf"
	"github.com/OpenCHAMI/smd/v2/pkg/sm"
)

// TestData represents test data for state manager tests
type TestData struct {
	Components []SMComponent
	Groups     []Group
	IPAddrs    map[string]sm.CompEthInterfaceV2
}

// createTestServer creates a test HTTP server that serves mock HSM data
func createTestServer(t *testing.T, testData TestData) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hsm/v2/State/Components":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"Components": testData.Components,
			})
		case "/hsm/v2/groups":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(testData.Groups)
		case "/hsm/v2/Inventory/ComponentEndpoints":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sm.ComponentEndpointArray{
				ComponentEndpoints: []*sm.ComponentEndpoint{
					{
						ComponentDescription: rf.ComponentDescription{
							ID: "x1000c0s0b0n0",
						},
						Enabled:               true,
						RfEndpointFQDN:        "test-node.example.com",
						ComponentEndpointType: sm.CompEPTypeSystem,
						RedfishSystemInfo: &rf.ComponentSystemInfo{
							EthNICInfo: []*rf.EthernetNICInfo{
								{MACAddress: "00:11:22:33:44:55"},
							},
						},
					},
				},
			})
		case "/hsm/v2/Inventory/EthernetInterfaces":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]sm.CompEthInterfaceV2{
				{
					CompID:  "x1000c0s0b0n0",
					MACAddr: "00:11:22:33:44:55",
					IPAddrs: []sm.IPAddressMapping{
						{IPAddr: "10.0.0.1"},
					},
				},
			})
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
}

// createTestFile creates a temporary file with test data
func createTestFile(t *testing.T, testData TestData) string {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_state.json")

	data := SMData{
		Components: testData.Components,
		Groups:     testData.Groups,
		IPAddrs:    testData.IPAddrs,
	}

	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(data); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	return filePath
}

// testStateManager runs tests against a StateManager implementation
func testStateManager(t *testing.T, sm StateManager) {
	// Test data
	testComp := SMComponent{
		Component: base.Component{
			ID:  "x1000c0s0b0n0",
			NID: json.Number("1"),
		},
		Mac:             []string{"00:11:22:33:44:55"},
		EndpointEnabled: true,
		Fqdn:            "test-node.example.com",
	}

	testGroup := Group{
		GroupName:    "test-group",
		GroupType:    "Node",
		GroupTags:    []string{"test"},
		GroupMembers: []string{"x1000c0s0b0n0"},
	}

	// Test GetState
	t.Run("GetState", func(t *testing.T) {
		state, err := sm.GetState()
		if err != nil {
			t.Errorf("GetState failed: %v", err)
			return
		}
		if state == nil {
			t.Error("GetState returned nil state")
		}
	})

	// Test GetComponentByName
	t.Run("GetComponentByName", func(t *testing.T) {
		comp, found := sm.GetComponentByName(testComp.ID)
		if !found {
			t.Error("GetComponentByName failed to find component")
			return
		}
		if comp.ID != testComp.ID {
			t.Errorf("GetComponentByName returned wrong component: got %s, want %s", comp.ID, testComp.ID)
		}
	})

	// Test GetComponentByMAC
	t.Run("GetComponentByMAC", func(t *testing.T) {
		comp, found := sm.GetComponentByMAC(testComp.Mac[0])
		if !found {
			t.Error("GetComponentByMAC failed to find component")
			return
		}
		if comp.ID != testComp.ID {
			t.Errorf("GetComponentByMAC returned wrong component: got %s, want %s", comp.ID, testComp.ID)
		}
	})

	// Test GetComponentByNID
	t.Run("GetComponentByNID", func(t *testing.T) {
		comp, found := sm.GetComponentByNID(1)
		if !found {
			t.Error("GetComponentByNID failed to find component")
			return
		}
		if comp.ID != testComp.ID {
			t.Errorf("GetComponentByNID returned wrong component: got %s, want %s", comp.ID, testComp.ID)
		}
	})

	// Test GetGroupsByMAC
	t.Run("GetGroupsByMAC", func(t *testing.T) {
		groups, found := sm.GetGroupsByMAC(testComp.Mac[0])
		if !found {
			t.Error("GetGroupsByMAC failed to find groups")
			return
		}
		if len(groups) == 0 {
			t.Error("GetGroupsByMAC returned empty groups slice")
			return
		}
		if groups[0].GroupName != testGroup.GroupName {
			t.Errorf("GetGroupsByMAC returned wrong group: got %s, want %s", groups[0].GroupName, testGroup.GroupName)
		}
	})

	// Test GetGroupsByName
	t.Run("GetGroupsByName", func(t *testing.T) {
		groups, found := sm.GetGroupsByName(testComp.ID)
		if !found {
			t.Error("GetGroupsByName failed to find groups")
			return
		}
		if len(groups) == 0 {
			t.Error("GetGroupsByName returned empty groups slice")
			return
		}
		if groups[0].GroupName != testGroup.GroupName {
			t.Errorf("GetGroupsByName returned wrong group: got %s, want %s", groups[0].GroupName, testGroup.GroupName)
		}
	})

	// Test RefreshState
	t.Run("RefreshState", func(t *testing.T) {
		if err := sm.RefreshState(); err != nil {
			t.Errorf("RefreshState failed: %v", err)
		}
	})
}

func TestHSMStateManager(t *testing.T) {
	// Create test data
	testData := TestData{
		Components: []SMComponent{
			{
				Component: base.Component{
					ID:  "x1000c0s0b0n0",
					NID: json.Number("1"),
				},
				Mac:             []string{"00:11:22:33:44:55"},
				EndpointEnabled: true,
				Fqdn:            "test-node.example.com",
			},
		},
		Groups: []Group{
			{
				GroupName:    "test-group",
				GroupType:    "Node",
				GroupTags:    []string{"test"},
				GroupMembers: []string{"x1000c0s0b0n0"},
			},
		},
		IPAddrs: map[string]sm.CompEthInterfaceV2{
			"10.0.0.1": {
				CompID:  "x1000c0s0b0n0",
				MACAddr: "00:11:22:33:44:55",
			},
		},
	}

	// Create test server
	server := createTestServer(t, testData)
	defer server.Close()

	// Only initialize if not already initialized
	if stateManager == nil {
		if err := SmOpen("mem:", ""); err != nil {
			t.Fatalf("Failed to initialize state manager: %v", err)
		}
	}

	// Run tests
	testStateManager(t, stateManager)
}

func TestFileStateManager(t *testing.T) {
	// Create test data
	testData := TestData{
		Components: []SMComponent{
			{
				Component: base.Component{
					ID:  "x1000c0s0b0n0",
					NID: json.Number("1"),
				},
				Mac:             []string{"00:11:22:33:44:55"},
				EndpointEnabled: true,
				Fqdn:            "test-node.example.com",
			},
		},
		Groups: []Group{
			{
				GroupName:    "test-group",
				GroupType:    "Node",
				GroupTags:    []string{"test"},
				GroupMembers: []string{"x1000c0s0b0n0"},
			},
		},
		IPAddrs: map[string]sm.CompEthInterfaceV2{
			"10.0.0.1": {
				CompID:  "x1000c0s0b0n0",
				MACAddr: "00:11:22:33:44:55",
			},
		},
	}

	// Create test file
	filePath := createTestFile(t, testData)

	// Only initialize if not already initialized
	if stateManager == nil {
		if err := SmOpen("file:"+filePath, ""); err != nil {
			t.Fatalf("Failed to initialize state manager: %v", err)
		}
	}

	// Run tests
	testStateManager(t, stateManager)
}
