package helmutil

import (
	"testing"

	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestLoadRepositoryCatalogFromOCIRepository(t *testing.T) {
	repository := model.HelmRepository{
		Name: "mplatform",
		URL:  "oci://ghcr.io/qzytwway/charts/mplatform",
	}

	indexFile, err := LoadRepositoryCatalog(repository)
	require.NoError(t, err)

	entries := indexFile.Entries["mplatform"]
	require.NotEmpty(t, entries)
	require.Equal(t, "mplatform", entries[0].Name)
	require.NotEmpty(t, entries[0].Version)
	require.NotEmpty(t, entries[0].URLs)
	require.Equal(t, repository.URL+":"+entries[0].Version, entries[0].URLs[0])
}
