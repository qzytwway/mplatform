package helmutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/require"
)

func newFakeHarbor(t *testing.T) *httptest.Server {
	t.Helper()
	chartArtifact := func(extraAttrs string, tags ...string) map[string]any {
		tagItems := make([]map[string]string, 0, len(tags))
		for _, tag := range tags {
			tagItems = append(tagItems, map[string]string{"name": tag})
		}
		return map[string]any{
			"type":        "CHART",
			"push_time":   "2026-01-02T03:04:05Z",
			"extra_attrs": json.RawMessage(extraAttrs),
			"tags":        tagItems,
		}
	}
	imageArtifact := map[string]any{"type": "IMAGE", "tags": []map[string]string{{"name": "1.0.0"}}}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if username, password, ok := r.BasicAuth(); !ok || username != "robot$apps" || password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body any
		switch path := r.URL.EscapedPath(); {
		case path == "/api/v2.0/projects/apps/repositories":
			// The first page is full, so the client must ask for page 2.
			if r.URL.Query().Get("page") == "1" {
				repositories := make([]map[string]string, 0, harborPageSize)
				for i := range harborPageSize {
					repositories = append(repositories, map[string]string{"name": fmt.Sprintf("apps/image-%d", i)})
				}
				body = repositories
			} else {
				body = []map[string]string{{"name": "apps/nginx"}, {"name": "apps/team/app"}}
			}
		case path == "/api/v2.0/projects/apps/repositories/nginx/artifacts":
			body = []map[string]any{
				chartArtifact(`{"name":"nginx","version":"1.2.0","description":"Web server","appVersion":"1.25"}`, "1.2.0", "latest"),
				chartArtifact(`{"name":"nginx","version":"1.10.0","description":"Web server"}`, "1.10.0"),
			}
		case path == "/api/v2.0/projects/apps/repositories/team%252Fapp/artifacts":
			body = []map[string]any{chartArtifact(`{"name":"app","version":"0.1.0"}`, "0.1.0")}
		case strings.HasPrefix(path, "/api/v2.0/projects/apps/repositories/image-"):
			body = []map[string]any{imageArtifact}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(body))
	}))
}

func TestLoadRepositoryCatalogFromHarborProject(t *testing.T) {
	server := newFakeHarbor(t)
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	repository := model.HelmRepository{
		Name:      "harbor-apps",
		Type:      model.HelmRepositoryTypeHarbor,
		URL:       "oci://" + host + "/apps",
		PlainHTTP: true,
		Username:  "robot$apps",
		Password:  "secret",
	}

	indexFile, err := LoadRepositoryCatalog(repository)
	require.NoError(t, err)

	require.Len(t, indexFile.Entries, 2)
	nginx := indexFile.Entries["nginx"]
	require.Len(t, nginx, 2)
	require.Equal(t, "1.10.0", nginx[0].Version)
	require.Equal(t, []string{"oci://" + host + "/apps/nginx:1.10.0"}, nginx[0].URLs)
	require.Equal(t, "1.2.0", nginx[1].Version)
	require.Equal(t, "Web server", nginx[1].Description)
	require.Equal(t, "1.25", nginx[1].AppVersion)
	require.False(t, nginx[1].Created.IsZero())

	app := indexFile.Entries["app"]
	require.Len(t, app, 1)
	require.Equal(t, []string{"oci://" + host + "/apps/team/app:0.1.0"}, app[0].URLs)
}

func TestLoadRepositoryCatalogFromHarborProjectRejectsBadCredentials(t *testing.T) {
	server := newFakeHarbor(t)
	defer server.Close()
	repository := model.HelmRepository{
		Name:      "harbor-apps",
		Type:      model.HelmRepositoryTypeHarbor,
		URL:       "oci://" + strings.TrimPrefix(server.URL, "http://") + "/apps",
		PlainHTTP: true,
		Username:  "robot$apps",
		Password:  "wrong",
	}

	_, err := LoadRepositoryCatalog(repository)
	require.ErrorContains(t, err, "401")
}
