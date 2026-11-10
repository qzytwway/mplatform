package helmutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
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

// imageRegistryPostRenderer moves every container image under imageRegistry,
// keeping only the image name, tag and digest.
type imageRegistryPostRenderer struct {
	imageRegistry string
}

func (r imageRegistryPostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
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
		r.rewriteImages(&document)
		if err := encoder.Encode(&document); err != nil {
			return nil, fmt.Errorf("failed to write rendered manifests: %w", err)
		}
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r imageRegistryPostRenderer) rewriteImages(node *yaml.Node) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if containerListKeys[key.Value] && value.Kind == yaml.SequenceNode {
				for _, container := range value.Content {
					r.rewriteContainerImage(container)
				}
			}
		}
	}
	for _, child := range node.Content {
		r.rewriteImages(child)
	}
}

func (r imageRegistryPostRenderer) rewriteContainerImage(container *yaml.Node) {
	if container.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(container.Content); i += 2 {
		if container.Content[i].Value == "image" && container.Content[i+1].Kind == yaml.ScalarNode {
			container.Content[i+1].Value = rewriteImageRegistry(container.Content[i+1].Value, r.imageRegistry)
		}
	}
}

// rewriteImageRegistry returns image under imageRegistry. Images already
// served by the registry's host, and unparsable references, are unchanged.
func rewriteImageRegistry(image, imageRegistry string) string {
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
