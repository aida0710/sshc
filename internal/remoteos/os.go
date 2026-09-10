// Package remoteos normalizes remote OS names for presentation only.
package remoteos

import "strings"

func Valid(value string) bool {
	switch value {
	case "", "server", "linux", "amazonlinux", "ubuntu", "debian", "redhat", "fedora", "centos", "rocky", "almalinux", "arch", "alpine", "opensuse", "freebsd", "macos", "windows":
		return true
	}
	return false
}

func normalize(value string) string {
	value = strings.ToLower(strings.Trim(value, " \t\r\"'"))
	switch value {
	case "amzn":
		return "amazonlinux"
	case "rhel":
		return "redhat"
	case "archlinux":
		return "arch"
	case "opensuse-leap", "opensuse-tumbleweed", "sles", "sled":
		return "opensuse"
	case "darwin":
		return "macos"
	}
	if Valid(value) && value != "server" {
		return value
	}
	return ""
}

// Parse never executes os-release: it reads only ID and ID_LIKE as text.
func Parse(output string) string {
	var kernel, id, like string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "ID="); ok {
			id = normalize(value)
		}
		if value, ok := strings.CutPrefix(line, "ID_LIKE="); ok {
			for _, candidate := range strings.Fields(strings.Trim(value, "\"'")) {
				if like == "" {
					like = normalize(candidate)
				}
			}
		}
		switch line {
		case "Linux":
			kernel = "linux"
		case "Darwin":
			kernel = "macos"
		case "FreeBSD":
			kernel = "freebsd"
		}
	}
	if id != "" {
		return id
	}
	if like != "" {
		return like
	}
	return kernel
}
