package helmutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	semver "github.com/blang/semver/v4"
	"github.com/qzytwway/mplatform/pkg/model"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/getter"
	repo "helm.sh/helm/v4/pkg/repo/v1"
	"k8s.io/klog/v2"
)

const (
	harborPageSize          = 100
	harborArtifactTypeChart = "CHART"
)

var harborHTTPClient = &http.Client{Timeout: time.Duration(getter.DefaultHTTPTimeout) * time.Second}

type harborRepository struct {
	Name string `json:"name"`
}

type harborArtifact struct {
	Type       string          `json:"type"`
	PushTime   *time.Time      `json:"push_time"`
	ExtraAttrs json.RawMessage `json:"extra_attrs"`
	Tags       []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

// loadHarborRepositoryIndex lists every chart in a Harbor project. Harbor
// restricts the registry catalog to system admins, so the project API is used.
func loadHarborRepositoryIndex(repository model.HelmRepository) (*repo.IndexFile, error) {
	repositoryURL, err := url.Parse(repository.URL)
	if err != nil {
		return nil, err
	}
	project := strings.Trim(repositoryURL.Path, "/")
	scheme := "https"
	if repository.PlainHTTP {
		scheme = "http"
	}
	projectAPI := scheme + "://" + repositoryURL.Host + "/api/v2.0/projects/" + url.PathEscape(project)

	repositories, err := listHarbor[harborRepository](repository, projectAPI+"/repositories")
	if err != nil {
		return nil, err
	}

	indexFile := &repo.IndexFile{Entries: map[string]repo.ChartVersions{}}
	for _, harborRepo := range repositories {
		// Harbor wants slashes in repository names encoded twice.
		repoName := strings.TrimPrefix(harborRepo.Name, project+"/")
		artifacts, err := listHarbor[harborArtifact](repository, projectAPI+"/repositories/"+url.PathEscape(url.PathEscape(repoName))+"/artifacts")
		if err != nil {
			return nil, err
		}
		versions, err := harborChartVersions(repositoryURL.Host, harborRepo.Name, artifacts)
		if err != nil {
			return nil, err
		}
		if len(versions) == 0 {
			continue
		}
		chartName := versions[0].Name
		if _, exists := indexFile.Entries[chartName]; exists {
			klog.Warningf("Skipping Harbor repository %s: chart %s already listed in %s", harborRepo.Name, chartName, repository.URL)
			continue
		}
		indexFile.Entries[chartName] = versions
	}
	return indexFile, nil
}

func harborChartVersions(host, repositoryName string, artifacts []harborArtifact) (repo.ChartVersions, error) {
	var versions repo.ChartVersions
	for _, artifact := range artifacts {
		if !strings.EqualFold(artifact.Type, harborArtifactTypeChart) {
			continue
		}
		var metadata chart.Metadata
		if len(artifact.ExtraAttrs) > 0 {
			if err := json.Unmarshal(artifact.ExtraAttrs, &metadata); err != nil {
				return nil, fmt.Errorf("failed to parse chart metadata of %s: %w", repositoryName, err)
			}
		}
		if metadata.Name == "" {
			metadata.Name = path.Base(repositoryName)
		}
		for _, tag := range artifact.Tags {
			if _, err := semver.ParseTolerant(tag.Name); err != nil {
				continue
			}
			tagMetadata := metadata
			tagMetadata.Version = tag.Name
			version := &repo.ChartVersion{
				Metadata: &tagMetadata,
				URLs:     []string{"oci://" + host + "/" + repositoryName + ":" + tag.Name},
			}
			if artifact.PushTime != nil {
				version.Created = *artifact.PushTime
			}
			versions = append(versions, version)
		}
	}
	sort.SliceStable(versions, func(i, j int) bool {
		return CompareChartVersions(versions[i].Version, versions[j].Version) > 0
	})
	return versions, nil
}

func listHarbor[T any](repository model.HelmRepository, endpoint string) ([]T, error) {
	var items []T
	for page := 1; ; page++ {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s?page=%d&page_size=%d", endpoint, page, harborPageSize), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if repository.Username != "" {
			req.SetBasicAuth(repository.Username, string(repository.Password))
		}
		pageItems, err := doHarborRequest[T](req)
		if err != nil {
			return nil, err
		}
		items = append(items, pageItems...)
		if len(pageItems) < harborPageSize {
			return items, nil
		}
	}
}

func doHarborRequest[T any](req *http.Request) ([]T, error) {
	resp, err := harborHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("harbor request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("harbor request %s failed: %s", req.URL.Path, resp.Status)
	}
	var items []T
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode harbor response from %s: %w", req.URL.Path, err)
	}
	return items, nil
}
