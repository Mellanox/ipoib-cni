package config

import (
	"encoding/json"
	"fmt"

	"github.com/Mellanox/ipoib-cni/pkg/types"
)

// LoadConf parses and validates stdin netconf and returns NetConf object
func LoadConf(bytes []byte) (*types.NetConf, string, error) {
	n := &types.NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, "", fmt.Errorf("failed to load netconf: %v", err)
	}
	if n.Master == "" {
		return nil, "", fmt.Errorf("host master interface is missing")
	}
	return n, n.CNIVersion, nil
}
