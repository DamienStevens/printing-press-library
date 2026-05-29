// Package cred resolves per-source credentials from the macOS Keychain, with
// an environment-variable fallback. Home Health is macOS-only (its sensors are
// on the user's LAN and the canonical credential store is the login Keychain),
// so shelling out to /usr/bin/security is the supported path — the same binary
// that created the items reads them back without a GUI authorization prompt.
//
// Credential layout (created by the user via `security add-generic-password`):
//
//	service                      account            secret
//	home-health-mocreo           <mocreo-email>     <mocreo-password>
//	home-health-airthings        <client_id>        <client_secret>
//	home-health-iqair            <iqair-email>      <iqair-password>
//	home-health-airvisual-smb    <pro-ip>           <smb-password>
//
// The account field holds the non-secret identifier (email / client_id / ip);
// the password field holds the secret. Env-var overrides use the SCREAMING
// form of the service plus _ACCOUNT / _SECRET (e.g. HOME_HEALTH_MOCREO_ACCOUNT).
package cred

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Service constants — keep in lockstep with the documented Keychain layout.
const (
	ServiceMOCREO       = "home-health-mocreo"
	ServiceAirThings    = "home-health-airthings"
	ServiceIQAir        = "home-health-iqair"
	ServiceAirVisualSMB = "home-health-airvisual-smb"
)

// Credential is the resolved (account, secret) pair for one service.
type Credential struct {
	Account string // email / client_id / ip — safe to log
	Secret  string // password / client_secret — never log
}

// Get resolves a credential, preferring env-var overrides, then the Keychain.
// Returns a credential with Found=false (nil error) when neither source has it,
// so callers can report "not configured" per source rather than hard-failing.
func Get(ctx context.Context, service string) (Credential, bool, error) {
	envBase := envPrefix(service)
	acct := os.Getenv(envBase + "_ACCOUNT")
	secret := os.Getenv(envBase + "_SECRET")
	if secret != "" {
		return Credential{Account: acct, Secret: secret}, true, nil
	}

	// Keychain: one call for the secret (-w), one for the account metadata.
	secret, err := securityFind(ctx, service, true)
	if err != nil {
		if isNotFound(err) {
			return Credential{}, false, nil
		}
		return Credential{}, false, err
	}
	if secret == "" {
		return Credential{}, false, nil
	}
	if acct == "" {
		acct, _ = securityAccount(ctx, service)
	}
	return Credential{Account: acct, Secret: secret}, true, nil
}

// envPrefix maps "home-health-mocreo" -> "HOME_HEALTH_MOCREO".
func envPrefix(service string) string {
	return strings.ToUpper(strings.ReplaceAll(service, "-", "_"))
}

// securityFind runs `security find-generic-password -s <service> [-w]`.
// With wantSecret it returns the raw secret; otherwise the full record.
func securityFind(ctx context.Context, service string, wantSecret bool) (string, error) {
	args := []string{"find-generic-password", "-s", service}
	if wantSecret {
		args = append(args, "-w")
	}
	out, err := exec.CommandContext(ctx, "/usr/bin/security", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// securityAccount parses the "acct" attribute out of the full record.
func securityAccount(ctx context.Context, service string) (string, error) {
	out, err := exec.CommandContext(ctx, "/usr/bin/security",
		"find-generic-password", "-s", service).CombinedOutput()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `"acct"`) {
			if i := strings.Index(line, `="`); i >= 0 {
				return strings.TrimSuffix(line[i+2:], `"`), nil
			}
		}
	}
	return "", fmt.Errorf("cred: no acct attribute for service %q", service)
}

// isNotFound reports whether the error is `security`'s "item not found" exit
// (exit status 44), as opposed to a real failure.
func isNotFound(err error) bool {
	var ee *exec.ExitError
	if ok := asExitError(err, &ee); ok {
		return ee.ExitCode() == 44
	}
	return false
}

func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}
