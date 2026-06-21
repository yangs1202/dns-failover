package dnsprovider

import "strings"

func ensureTrailingDot(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.TrimSuffix(name, ".")
	return name + "."
}

func relativeDNSName(name string, zoneName string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	zoneName = strings.TrimSuffix(strings.TrimSpace(zoneName), ".")
	if name == zoneName {
		return "@"
	}
	suffix := "." + zoneName
	if strings.HasSuffix(name, suffix) {
		return strings.TrimSuffix(name, suffix)
	}
	return name
}
