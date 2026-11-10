package utils

import (
	"fmt"
	"html"
	"net"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/validation"
)

const mplatformBasePlaceholder = "__MPLATFORM_BASE__"

func InjectMPlatformBase(htmlContent string, base string) string {
	assetBase := base
	if assetBase == "/" {
		assetBase = ""
	}

	htmlContent = strings.ReplaceAll(htmlContent, mplatformBasePlaceholder, html.EscapeString(assetBase))

	baseScript := fmt.Sprintf(`<script>window.__dynamic_base__=%q;</script>`, assetBase)
	re := regexp.MustCompile(`<head>`)
	return re.ReplaceAllString(htmlContent, "<head>\n    "+baseScript)
}

func RandomString(length int) string {
	return rand.String(length)
}

func GetImageRegistryAndRepo(image string) (string, string) {
	if digestIndex := strings.Index(image, "@"); digestIndex >= 0 {
		image = image[:digestIndex]
	}
	if tagIndex := strings.LastIndex(image, ":"); tagIndex > strings.LastIndex(image, "/") {
		image = image[:tagIndex]
	}
	parts := strings.Split(image, "/")
	if len(parts) == 1 {
		return "", "library/" + parts[0]
	}
	if len(parts) > 1 {
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			return parts[0], strings.Join(parts[1:], "/")
		}
		return "", strings.Join(parts, "/")
	}
	return "", image
}

const (
	minNodePort = 30000
	maxNodePort = 32767
)

// NodeEndpoint validates the address of a node and a NodePort on it, and
// joins them as host:port.
func NodeEndpoint(host string, port int) (string, error) {
	if port < minNodePort || port > maxNodePort {
		return "", fmt.Errorf("node port must be between %d and %d", minNodePort, maxNodePort)
	}
	host = strings.TrimSpace(host)
	if net.ParseIP(host) == nil && len(validation.IsDNS1123Subdomain(host)) > 0 {
		return "", fmt.Errorf("node address must be an IP address or host name")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
