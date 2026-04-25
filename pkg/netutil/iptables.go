package netutil

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ipsetNameRe validates ipset names: alphanumeric, underscores, hyphens only.
var ipsetNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// allowedActions is the whitelist of valid iptables actions.
var allowedActions = map[string]bool{"DROP": true, "ACCEPT": true, "REJECT": true}

// IptablesManager wraps iptables/ipset commands.
type IptablesManager struct{}

// NewIptablesManager creates a new IptablesManager.
func NewIptablesManager() *IptablesManager {
	return &IptablesManager{}
}

// ValidateIPSetName checks that a name is safe for ipset operations.
func ValidateIPSetName(name string) error {
	if name == "" {
		return fmt.Errorf("ipset name must not be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("ipset name too long: %d (max 128)", len(name))
	}
	if !ipsetNameRe.MatchString(name) {
		return fmt.Errorf("ipset name contains invalid characters: %q", name)
	}
	return nil
}

// ValidatePort checks that a port string is a valid TCP port number (1-65535).
func ValidatePort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port: %q", port)
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("port out of range: %d (must be 1-65535)", n)
	}
	return nil
}

// ValidateAction checks that an action is a whitelisted iptables jump target.
func ValidateAction(action string) error {
	if !allowedActions[action] {
		return fmt.Errorf("invalid iptables action: %q (allowed: DROP, ACCEPT, REJECT)", action)
	}
	return nil
}

// CreateIPSet creates an ipset with hash:net family inet.
func (m *IptablesManager) CreateIPSet(name string, maxElem int) error {
	if err := ValidateIPSetName(name); err != nil {
		return err
	}
	return exec.Command("ipset", "create", "-exist", name,
		"hash:net", "family", "inet", "maxelem", fmt.Sprintf("%d", maxElem)).Run()
}

// FlushIPSet flushes all entries from an ipset.
func (m *IptablesManager) FlushIPSet(name string) error {
	if err := ValidateIPSetName(name); err != nil {
		return err
	}
	return exec.Command("ipset", "flush", name).Run()
}

// RestoreIPSet loads CIDR entries via ipset restore.
func (m *IptablesManager) RestoreIPSet(name string, cidrs []string) error {
	if err := ValidateIPSetName(name); err != nil {
		return err
	}
	var b strings.Builder
	for _, cidr := range cidrs {
		// Validate CIDR to prevent ipset command injection
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		b.WriteString(fmt.Sprintf("add %s %s\n", name, cidr))
	}
	cmd := exec.Command("ipset", "restore", "-exist")
	cmd.Stdin = strings.NewReader(b.String())
	return cmd.Run()
}

// DestroyIPSet destroys an ipset.
func (m *IptablesManager) DestroyIPSet(name string) error {
	if err := ValidateIPSetName(name); err != nil {
		return err
	}
	return exec.Command("ipset", "destroy", name).Run()
}

// SetRule creates an iptables rule for geo-blocking.
// action: "DROP" or "ACCEPT"
func (m *IptablesManager) SetRule(setName, port, action string) error {
	if err := ValidateIPSetName(setName); err != nil {
		return err
	}
	if err := ValidatePort(port); err != nil {
		return err
	}
	if err := ValidateAction(action); err != nil {
		return err
	}

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
	if err := ValidatePort(port); err != nil {
		return err
	}

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
		if !strings.Contains(line, "popugate-geoblock") {
			continue
		}
		// Only process lines that look like iptables rules (start with -A)
		if !strings.HasPrefix(strings.TrimSpace(line), "-A ") {
			continue
		}
		// Convert -A to -D for deletion
		rule := strings.Replace(line, "-A ", "-D ", 1)
		parts := strings.Fields(rule)
		if len(parts) > 1 && parts[0] == "-D" {
			_ = exec.Command("iptables", parts[1:]...).Run()
		}
	}
	return nil
}

// SetNameForCountry generates the ipset name for a country code.
func SetNameForCountry(code string) string {
	return fmt.Sprintf("mtpmax_%s", strings.ToLower(code))
}
