package ai

import (
	"context"
	"testing"
)

func TestNormalizeNamespace(t *testing.T) {
	if got := normalizeNamespace(resourceInfo{ClusterScoped: true}, "default"); got != "" {
		t.Fatalf("expected empty namespace for cluster-scoped resource, got %q", got)
	}
	if got := normalizeNamespace(resourceInfo{}, "default"); got != "default" {
		t.Fatalf("expected namespace to pass through, got %q", got)
	}
}

func TestBuildObjectForResource(t *testing.T) {
	obj := buildObjectForResource(resourceInfo{Kind: "Pod", Group: "", Version: "v1"})
	gvk := obj.GroupVersionKind()
	if gvk.Kind != "Pod" || gvk.Version != "v1" {
		t.Fatalf("unexpected GVK: %#v", gvk)
	}
}

func TestResolveResourceInfoFallsBackToStaticMapping(t *testing.T) {
	info := resolveResourceInfo(context.Background(), nil, "pods")
	if info.Kind != "Pod" || info.Resource != "pods" {
		t.Fatalf("unexpected resource info: %#v", info)
	}
}

func TestGetRequiredString(t *testing.T) {
	got, err := getRequiredString(map[string]interface{}{"name": "  example  "}, "name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "example" {
		t.Fatalf("unexpected value: %q", got)
	}

	if _, err := getRequiredString(map[string]interface{}{}, "name"); err == nil {
		t.Fatalf("expected error for missing required string")
	}
}
