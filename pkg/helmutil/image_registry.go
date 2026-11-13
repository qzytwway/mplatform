package helmutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/distribution/reference"
	"github.com/qzytwway/mplatform/pkg/model"
	"gopkg.in/yaml.v3"
)

var containerListKeys = map[string]bool{
	"containers":          true,
	"initContainers":      true,
	"ephemeralContainers": true,
}

// imageArgPrefixes are container arguments that name images the container
// starts later, such as the sidecars prometheus-operator injects.
var imageArgPrefixes = []string{
	"--prometheus-config-reloader=",
	"--thanos-default-base-image=",
}

// imageEnvNames are container environment variables that name images the
// container starts later, such as the GPU Operator's driver manager.
var imageEnvNames = map[string]bool{
	"DRIVER_MANAGER_IMAGE": true,
	"VALIDATOR_IMAGE":      true,
}

// monitoringAPIGroup resources (Prometheus, Alertmanager, ThanosRuler...)
// carry the image of the pods the operator creates in spec.image.
const monitoringAPIGroup = "monitoring.coreos.com/"

// The GPU Operator's ClusterPolicy names the image of each component it
// deploys by separate repository, image and version fields.
const (
	clusterPolicyAPIVersion = "nvidia.com/v1"
	clusterPolicyKind       = "ClusterPolicy"
)

// ValidateImageRegistry accepts a registry host with an optional path prefix,
// such as harbor.example.com or harbor.example.com:8443/mirror.
func ValidateImageRegistry(imageRegistry string) error {
	if imageRegistry == "" {
		return nil
	}
	if _, err := reference.ParseNamed(imageRegistry + "/image"); err != nil {
		return fmt.Errorf("image registry must be a registry host with an optional path, e.g. harbor.example.com/library")
	}
	return nil
}

// ClusterImageRegistry returns the image registry configured for a cluster.
func ClusterImageRegistry(clusterName string) (string, error) {
	cluster, err := model.GetClusterByName(clusterName)
	if err != nil {
		return "", fmt.Errorf("failed to load image registry of cluster %s: %w", clusterName, err)
	}
	return cluster.ImageRegistry, nil
}

// imageRegistryPostRenderer moves every container image, image argument and
// environment variable, prometheus-operator resource image and GPU Operator
// ClusterPolicy image under imageRegistry, keeping only the image name, tag
// and digest.
type imageRegistryPostRenderer struct {
	imageRegistry string
}

// Run also moves the images of ClusterPolicy components that are turned off,
// so they are pulled from the registry if they are turned on later.
func (r imageRegistryPostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	return visitManifestImages(renderedManifests, false, func(image string) string {
		return RewriteImageRegistry(image, r.imageRegistry)
	})
}

// imageCollector records the images imageRegistryPostRenderer moves, except
// those of ClusterPolicy components that are turned off and deploy nothing,
// and leaves the manifests unchanged.
type imageCollector struct {
	images []string
}

func (c *imageCollector) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	return visitManifestImages(renderedManifests, true, func(image string) string {
		if !slices.Contains(c.images, image) {
			c.images = append(c.images, image)
		}
		return image
	})
}

// visitManifestImages replaces every image the manifests deploy with what
// visit returns for it, skipping ClusterPolicy components that are turned off
// when skipDisabled is set.
func visitManifestImages(renderedManifests *bytes.Buffer, skipDisabled bool, visit func(image string) string) (*bytes.Buffer, error) {
	decoder := yaml.NewDecoder(renderedManifests)
	out := &bytes.Buffer{}
	encoder := yaml.NewEncoder(out)
	encoder.SetIndent(2)
	for {
		var document yaml.Node
		if err := decoder.Decode(&document); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to parse rendered manifests: %w", err)
		}
		visitContainerImages(&document, visit)
		visitMonitoringImage(&document, visit)
		visitClusterPolicy(&document, skipDisabled, visit)
		if err := encoder.Encode(&document); err != nil {
			return nil, fmt.Errorf("failed to write rendered manifests: %w", err)
		}
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

func visitContainerImages(node *yaml.Node, visit func(string) string) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if containerListKeys[key.Value] && value.Kind == yaml.SequenceNode {
				for _, container := range value.Content {
					visitContainer(container, visit)
				}
			}
		}
	}
	for _, child := range node.Content {
		visitContainerImages(child, visit)
	}
}

func visitContainer(container *yaml.Node, visit func(string) string) {
	if image := mappingValue(container, "image"); image != nil && image.Kind == yaml.ScalarNode {
		image.Value = visit(image.Value)
	}
	if env := mappingValue(container, "env"); env != nil && env.Kind == yaml.SequenceNode {
		for _, variable := range env.Content {
			name, value := mappingValue(variable, "name"), mappingValue(variable, "value")
			if name != nil && value != nil && value.Kind == yaml.ScalarNode && imageEnvNames[name.Value] {
				value.Value = visit(value.Value)
			}
		}
	}
	args := mappingValue(container, "args")
	if args == nil || args.Kind != yaml.SequenceNode {
		return
	}
	for _, arg := range args.Content {
		for _, prefix := range imageArgPrefixes {
			if image, ok := strings.CutPrefix(arg.Value, prefix); ok && arg.Kind == yaml.ScalarNode {
				arg.Value = prefix + visit(image)
			}
		}
	}
}

func visitMonitoringImage(document *yaml.Node, visit func(string) string) {
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return
	}
	resource := document.Content[0]
	apiVersion := mappingValue(resource, "apiVersion")
	if apiVersion == nil || !strings.HasPrefix(apiVersion.Value, monitoringAPIGroup) {
		return
	}
	if image := mappingValue(mappingValue(resource, "spec"), "image"); image != nil && image.Kind == yaml.ScalarNode {
		image.Value = visit(image.Value)
	}
}

func visitClusterPolicy(document *yaml.Node, skipDisabled bool, visit func(string) string) {
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return
	}
	resource := document.Content[0]
	apiVersion, kind := mappingValue(resource, "apiVersion"), mappingValue(resource, "kind")
	if apiVersion == nil || kind == nil || apiVersion.Value != clusterPolicyAPIVersion || kind.Value != clusterPolicyKind {
		return
	}
	visitRepositories(mappingValue(resource, "spec"), skipDisabled, visit)
}

// visitRepositories visits every repository, image and version under node,
// such as spec.driver and spec.driver.manager, as one image reference. A
// component turned off with enabled: false deploys nothing, and is skipped
// when skipDisabled is set.
func visitRepositories(node *yaml.Node, skipDisabled bool, visit func(string) string) {
	if node == nil {
		return
	}
	if enabled := mappingValue(node, "enabled"); skipDisabled && enabled != nil && enabled.Value == "false" {
		return
	}
	repository, image := mappingValue(node, "repository"), mappingValue(node, "image")
	if repository != nil && image != nil && repository.Kind == yaml.ScalarNode && image.Kind == yaml.ScalarNode && repository.Value != "" {
		name := repository.Value + "/" + image.Value
		// The operator pulls name:version, or name@version for a digest.
		suffix := ""
		if version := mappingValue(node, "version"); version != nil && version.Kind == yaml.ScalarNode && version.Value != "" {
			suffix = ":" + version.Value
			if strings.HasPrefix(version.Value, "sha256:") {
				suffix = "@" + version.Value
			}
		}
		visited := strings.TrimSuffix(visit(name+suffix), suffix)
		repository.Value, image.Value = path.Dir(visited), path.Base(visited)
	}
	for _, child := range node.Content {
		visitRepositories(child, skipDisabled, visit)
	}
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// RewriteImageRegistry returns image under imageRegistry. Images already
// served by the registry's host, and unparsable references, are unchanged.
func RewriteImageRegistry(image, imageRegistry string) string {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return image
	}
	registryHost, _, _ := strings.Cut(imageRegistry, "/")
	if strings.EqualFold(reference.Domain(named), registryHost) {
		return image
	}
	rewritten := imageRegistry + "/" + path.Base(reference.Path(named))
	if tagged, ok := named.(reference.Tagged); ok {
		rewritten += ":" + tagged.Tag()
	}
	if digested, ok := named.(reference.Digested); ok {
		rewritten += "@" + digested.Digest().String()
	}
	return rewritten
}
