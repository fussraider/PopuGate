package netutil

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/fussraider/PopuGate/pkg/logger"
)

var iptablesLog = logger.WithScope("iptables")

// ipsetNameRe validates ipset names: alphanumeric, underscores, hyphens only.
var ipsetNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// allowedActions is the whitelist of valid iptables actions.
var allowedActions = map[string]bool{"DROP": true, "ACCEPT": true, "REJECT": true}

// validChains are the chains we expect when parsing iptables-save output.
var validChains = map[string]bool{"INPUT": true, "OUTPUT": true, "FORWARD": true, "POSTROUTING": true, "PREROUTING": true}

// IptablesManager wraps iptables/ipset commands.
type IptablesManager struct{}

// NewIptablesManager creates a new IptablesManager.
func NewIptablesManager() *IptablesManager {
	return &IptablesManager{}
}

// runCmd executes a command, captures stderr, and logs errors.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		iptablesLog.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// runCmdOutput executes a command and returns its stdout, logging errors.
func runCmdOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		iptablesLog.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, stderr)
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, stderr)
	}
	return out, nil
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
	err := runCmd("ipset", "create", "-exist", name,
		"hash:net", "family", "inet", "maxelem", fmt.Sprintf("%d", maxElem))
	if err != nil {
		return fmt.Errorf("create ipset %q: %w", name, err)
	}
	iptablesLog.Debugf("created ipset %s (maxelem %d)", name, maxElem)
	return nil
}

// FlushIPSet flushes all entries from an ipset.
func (m *IptablesManager) FlushIPSet(name string) error {
	if err := ValidateIPSetName(name); err != nil {
		return err
	}
	err := runCmd("ipset", "flush", name)
	if err != nil {
		return fmt.Errorf("flush ipset %q: %w", name, err)
	}
	iptablesLog.Debugf("flushed ipset %s", name)
	return nil
}

// RestoreIPSet loads CIDR entries via ipset restore.
func (m *IptablesManager) RestoreIPSet(name string, cidrs []string) error {
	if err := ValidateIPSetName(name); err != nil {
		return err
	}
	var b strings.Builder
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		fmt.Fprintf(&b, "add %s %s\n", name, cidr)
	}
	cmd := exec.Command("ipset", "restore", "-exist")
	cmd.Stdin = strings.NewReader(b.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		iptablesLog.Errorf("ipset restore %s (%d entries): %v: %s", name, len(cidrs), err, strings.TrimSpace(string(out)))
		return fmt.Errorf("ipset restore %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	iptablesLog.Infof("restored ipset %s (%d entries)", name, len(cidrs))
	return nil
}

// DestroyIPSet destroys an ipset.
func (m *IptablesManager) DestroyIPSet(name string) error {
	if err := ValidateIPSetName(name); err != nil {
		return err
	}
	err := runCmd("ipset", "destroy", name)
	if err != nil {
		return fmt.Errorf("destroy ipset %q: %w", name, err)
	}
	iptablesLog.Debugf("destroyed ipset %s", name)
	return nil
}

// SetRule creates an iptables rule for geo-blocking.
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

	err := runCmd("iptables", "-I", "INPUT",
		"-m", "set", "--match-set", setName, "src",
		"-p", "tcp", "--dport", port,
		"-m", "comment", "--comment", "popugate-geoblock",
		"-j", action)
	if err != nil {
		return fmt.Errorf("set geoblock rule %s port %s %s: %w", setName, port, action, err)
	}
	iptablesLog.Infof("geoblock rule: set %s port %s → %s", setName, port, action)
	return nil
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

	err := runCmd("iptables", "-A", "INPUT",
		"-p", "tcp", "--dport", port,
		"-m", "comment", "--comment", "popugate-geoblock-default",
		"-j", "DROP")
	if err != nil {
		return fmt.Errorf("set default deny port %s: %w", port, err)
	}
	iptablesLog.Infof("geoblock default deny: port %s → DROP", port)
	return nil
}

// RemoveGeoBlockRules removes all popugate geo-block rules.
func (m *IptablesManager) RemoveGeoBlockRules() error {
	out, err := runCmdOutput("iptables-save")
	if err != nil {
		return fmt.Errorf("remove geoblock rules: %w", err)
	}

	removed := 0
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "popugate-geoblock") {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "-A ") {
			continue
		}
		rule := strings.Replace(line, "-A ", "-D ", 1)
		parts := strings.Fields(rule)
		if len(parts) < 3 || parts[0] != "-D" || !validChains[parts[1]] {
			iptablesLog.Warnf("skipping unexpected geoblock rule: %s", line)
			continue
		}
		if err := runCmd("iptables", parts...); err != nil {
			iptablesLog.Warnf("failed to remove geoblock rule: %v", err)
		} else {
			removed++
		}
	}
	iptablesLog.Infof("removed %d geoblock rules", removed)
	return nil
}

// SetNameForCountry generates the ipset name for a country code.
func SetNameForCountry(code string) string {
	return fmt.Sprintf("mtpmax_%s", strings.ToLower(code))
}

// SetTCPMSSRule applies a TCPMSS clamping rule in the mangle table for a given port.
func (m *IptablesManager) SetTCPMSSRule(port int, mss int) error {
	if err := ValidatePort(strconv.Itoa(port)); err != nil {
		return err
	}
	if mss < 1 || mss > 1460 {
		return fmt.Errorf("mss out of range: %d (must be 1-1460)", mss)
	}

	comment := fmt.Sprintf("popugate-tcpmss-%d", port)
	portStr := strconv.Itoa(port)
	mssStr := strconv.Itoa(mss)

	check := exec.Command("iptables", "-t", "mangle", "-C", "POSTROUTING",
		"-p", "tcp", "--sport", portStr,
		"--tcp-flags", "SYN,RST", "SYN",
		"-j", "TCPMSS", "--set-mss", mssStr,
		"-m", "comment", "--comment", comment)
	if check.Run() == nil {
		return nil
	}

	err := runCmd("iptables", "-t", "mangle", "-A", "POSTROUTING",
		"-p", "tcp", "--sport", portStr,
		"--tcp-flags", "SYN,RST", "SYN",
		"-j", "TCPMSS", "--set-mss", mssStr,
		"-m", "comment", "--comment", comment)
	if err != nil {
		return fmt.Errorf("set tcpmss rule port %d mss %d: %w", port, mss, err)
	}
	iptablesLog.Infof("tcpmss rule: port %d mss %d", port, mss)
	return nil
}

// RemoveTCPMSSRules removes TCPMSS rules from the mangle table.
// If port is 0, removes all popugate TCPMSS rules.
func (m *IptablesManager) RemoveTCPMSSRules(port int) error {
	out, err := runCmdOutput("iptables-save", "-t", "mangle")
	if err != nil {
		return fmt.Errorf("remove tcpmss rules: %w", err)
	}

	prefix := "popugate-tcpmss"
	if port > 0 {
		prefix = fmt.Sprintf("popugate-tcpmss-%d", port)
	}

	removed := 0
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, prefix) {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "-A ") {
			continue
		}
		rule := strings.Replace(line, "-A ", "-D ", 1)
		parts := strings.Fields(rule)
		if len(parts) < 3 || parts[0] != "-D" || !validChains[parts[1]] {
			iptablesLog.Warnf("skipping unexpected tcpmss rule: %s", line)
			continue
		}
		args := append([]string{"-t", "mangle"}, parts...)
		if err := runCmd("iptables", args...); err != nil {
			iptablesLog.Warnf("failed to remove tcpmss rule: %v", err)
		} else {
			removed++
		}
	}
	if removed > 0 || port > 0 {
		iptablesLog.Infof("removed %d tcpmss rules (port=%d)", removed, port)
	}
	return nil
}

// AddPortRedirect creates an iptables NAT redirect rule from primaryPort to tempPort.
func (m *IptablesManager) AddPortRedirect(primaryPort, tempPort int) error {
	if err := ValidatePort(strconv.Itoa(primaryPort)); err != nil {
		return err
	}
	if err := ValidatePort(strconv.Itoa(tempPort)); err != nil {
		return err
	}

	primaryStr := strconv.Itoa(primaryPort)
	tempStr := strconv.Itoa(tempPort)

	// Check if rule already exists
	check := exec.Command("iptables", "-t", "nat", "-C", "PREROUTING",
		"-p", "tcp", "--dport", primaryStr,
		"-j", "REDIRECT", "--to-ports", tempStr)
	if check.Run() == nil {
		return nil // rule exists
	}

	err := runCmd("iptables", "-t", "nat", "-A", "PREROUTING",
		"-p", "tcp", "--dport", primaryStr,
		"-j", "REDIRECT", "--to-ports", tempStr)
	if err != nil {
		return fmt.Errorf("add port redirect %d -> %d: %w", primaryPort, tempPort, err)
	}
	iptablesLog.Infof("port redirect rule added: %d -> %d", primaryPort, tempPort)
	return nil
}

// RemovePortRedirect deletes the iptables NAT redirect rule from primaryPort to tempPort.
func (m *IptablesManager) RemovePortRedirect(primaryPort, tempPort int) error {
	if err := ValidatePort(strconv.Itoa(primaryPort)); err != nil {
		return err
	}
	if err := ValidatePort(strconv.Itoa(tempPort)); err != nil {
		return err
	}

	primaryStr := strconv.Itoa(primaryPort)
	tempStr := strconv.Itoa(tempPort)

	// Check if rule exists before attempting delete to avoid errors
	check := exec.Command("iptables", "-t", "nat", "-C", "PREROUTING",
		"-p", "tcp", "--dport", primaryStr,
		"-j", "REDIRECT", "--to-ports", tempStr)
	if check.Run() != nil {
		return nil // rule does not exist
	}

	err := runCmd("iptables", "-t", "nat", "-D", "PREROUTING",
		"-p", "tcp", "--dport", primaryStr,
		"-j", "REDIRECT", "--to-ports", tempStr)
	if err != nil {
		return fmt.Errorf("remove port redirect %d -> %d: %w", primaryPort, tempPort, err)
	}
	iptablesLog.Infof("port redirect rule removed: %d -> %d", primaryPort, tempPort)
	return nil
}

// HasPortRedirect checks if there is an active iptables NAT redirect rule from primaryPort to tempPort.
func (m *IptablesManager) HasPortRedirect(primaryPort, tempPort int) (bool, error) {
	if err := ValidatePort(strconv.Itoa(primaryPort)); err != nil {
		return false, err
	}
	if err := ValidatePort(strconv.Itoa(tempPort)); err != nil {
		return false, err
	}

	primaryStr := strconv.Itoa(primaryPort)
	tempStr := strconv.Itoa(tempPort)

	check := exec.Command("iptables", "-t", "nat", "-C", "PREROUTING",
		"-p", "tcp", "--dport", primaryStr,
		"-j", "REDIRECT", "--to-ports", tempStr)
	err := check.Run()
	if err == nil {
		return true, nil
	}
	return false, nil
}

