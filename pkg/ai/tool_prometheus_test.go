package ai

import (
	"context"
	"testing"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/prometheus"
)

func TestExecuteQueryPrometheusRequiresClient(t *testing.T) {
	got, isError := executeQueryPrometheus(context.Background(), &cluster.ClientSet{}, map[string]interface{}{
		"query": "up",
	})
	if !isError {
		t.Fatalf("expected error result")
	}
	if got == "" {
		t.Fatalf("expected an error message")
	}
}

func TestExecuteQueryPrometheusRejectsUnsupportedQueryType(t *testing.T) {
	got, isError := executeQueryPrometheus(context.Background(), &cluster.ClientSet{
		PromClient: &prometheus.Client{},
	}, map[string]interface{}{
		"query":      "up",
		"query_type": "bad",
	})
	if !isError {
		t.Fatalf("expected error result")
	}
	if got != "Error: unsupported query_type 'bad'. Use 'instant' or 'range'." {
		t.Fatalf("unexpected error message: %q", got)
	}
}
