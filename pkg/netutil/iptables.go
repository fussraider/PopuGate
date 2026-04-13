package netutil

import (
	"fmt"
	"os/exec"
	"strings"
)

// IptablesManager wraps iptables/ipset commands.
type IptablesManager struct{}

// NewIptablesManager creates a new IptablesManager.
func NewIptablesManager() *IptablesManager {
	return &IptablesManager{}
}

// CreateIPSet creates an ipset with hash:net family inet.
func (m *IptablesManager) CreateIPSet(name string, maxElem int) error {
	return exec.Command("ipset", "create", "-exist", name,
		"hash:net", "family", "inet", "maxelem", fmt.Sprintf("%d", maxElem)).Run()
}

// FlushIPSet flushes all entries from an ipset.
func (m *IptablesManager) FlushIPSet(name string) error {
	return exec.Command("ipset", "flush", name).Run()
}

// RestoreIPSet loads CIDR entries via ipset restore.
func (m *IptablesManager) RestoreIPSet(name string, cidrs []string) error {
	var b strings.Builder
	for _, cidr := range cidrs {
		b.WriteString(fmt.Sprintf("add %s %s\n", name, cidr))
	}
	cmd := exec.Command("ipset", "restore", "-exist")
	cmd.Stdin = strings.NewReader(b.String())
	return cmd.Run()
}

// DestroyIPSet destroys an ipset.
func (m *IptablesManager) DestroyIPSet(name string) error {
	return exec.Command("ipset", "destroy", name).Run()
}

// SetRule creates an iptables rule for geo-blocking.
// action: "DROP" or "ACCEPT"
func (m *IptablesManager) SetRule(setName, port, action string) error {
	// Check if rule already exists
	check := exec.Command("iptables", "-C", "INPUT",
		"-m", "set", "--match-set", setName, "src",
		"-p", "tcp", "--dport", port,
		"-j", action)
	if check.Run() == nil {
		return nil // rule exists
	}

	return exec.Command("iptables", "-I", "INPUT",
		"-m", "set", "--match-set", setName, "src",
		"-p", "tcp", "--dport", port,
		"-m", "comment", "--comment", "popugate-geoblock",
		"-j", action).Run()
}

// SetDefaultDeny adds a default deny rule (for whitelist mode).
func (m *IptablesManager) SetDefaultDeny(port string) error {
	check := exec.Command("iptables", "-C", "INPUT",
		"-p", "tcp", "--dport", port,
		"-m", "comment", "--comment", "popugate-geoblock-default",
		"-j", "DROP")
	if check.Run() == nil {
		return nil
	}
	return exec.Command("iptables", "-A", "INPUT",
		"-p", "tcp", "--dport", port,
		"-m", "comment", "--comment", "popugate-geoblock-default",
		"-j", "DROP").Run()
}

// RemoveGeoBlockRules removes all popugate geo-block rules.
func (m *IptablesManager) RemoveGeoBlockRules() error {
	// List rules with comments
	out, err := exec.Command("iptables-save").Output()
	if err != nil {
		return fmt.Errorf("iptables-save: %w", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "popugate-geoblock") {
			// Convert -A to -D for deletion
			rule := strings.Replace(line, "-A ", "-D ", 1)
			parts := strings.Fields(rule)
			if len(parts) > 1 && parts[0] == "-D" {
				_ = exec.Command("iptables", parts[1:]...).Run()
			}
		}
	}
	return nil
}

// SetNameForCountry generates the ipset name for a country code.
func SetNameForCountry(code string) string {
	return fmt.Sprintf("mtpmax_%s", code)
}
