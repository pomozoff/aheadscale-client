package profilemanager

import (
	"fmt"
	"net/url"
	"strings"
)

// LockedServerOrigin is injected by the official package build. Empty keeps the
// upstream library usable by its development tools and existing unit fixtures.
var LockedServerOrigin string

func validateBuildOrigin(current *url.URL) error {
	if LockedServerOrigin == "" {
		return nil
	}
	expected, err := parseURL("Aheadscale build origin", LockedServerOrigin)
	if err != nil {
		return err
	}
	if current == nil || current.Scheme != expected.Scheme ||
		!strings.EqualFold(current.Hostname(), expected.Hostname()) ||
		originPort(current) != originPort(expected) || current.User != nil ||
		(current.Path != "" && current.Path != "/") || current.RawQuery != "" || current.Fragment != "" {
		return fmt.Errorf("profile belongs to another Aheadscale environment; use its original build or a clean test machine")
	}
	return nil
}

func originPort(origin *url.URL) string {
	if port := origin.Port(); port != "" {
		return port
	}
	if origin.Scheme == "https" {
		return "443"
	}
	return "80"
}
