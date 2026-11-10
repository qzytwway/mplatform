package helmutil

import (
	"context"
	"fmt"
	"strings"

	semver "github.com/blang/semver/v4"
	"github.com/qzytwway/mplatform/pkg/model"
)

const ChartSourceRepository = "repository"

type ChartPackage struct {
	Version    string
	URL        string
	Repository *model.HelmRepository
}

func ResolveChartRepository(repositoryName string) (*model.HelmRepository, error) {
	if repositoryName == "" {
		return nil, nil
	}
	var repository model.HelmRepository
	if err := model.DB.Where("name = ?", repositoryName).First(&repository).Error; err != nil {
		return nil, err
	}
	return &repository, nil
}

func LatestChartPackage(ctx context.Context, source, repositoryName, chartName string) (ChartPackage, error) {
	switch source {
	case "", ChartSourceRepository:
		return latestRepositoryChartPackage(repositoryName, chartName)
	default:
		return ChartPackage{}, fmt.Errorf("unsupported chart source")
	}
}

func latestRepositoryChartPackage(repositoryName, chartName string) (ChartPackage, error) {
	var repository model.HelmRepository
	if err := model.DB.Where("name = ?", repositoryName).First(&repository).Error; err != nil {
		return ChartPackage{}, err
	}
	indexFile, err := LoadRepositoryCatalog(repository)
	if err != nil {
		return ChartPackage{}, err
	}
	versions := indexFile.Entries[chartName]
	if len(versions) == 0 {
		return ChartPackage{}, fmt.Errorf("chart not found")
	}
	latest := versions[0]
	for _, version := range versions[1:] {
		if CompareChartVersions(version.Version, latest.Version) > 0 {
			latest = version
		}
	}
	if len(latest.URLs) == 0 {
		return ChartPackage{}, fmt.Errorf("chart package URL is missing")
	}
	return ChartPackage{
		Version:    latest.Version,
		URL:        ResolveURL(repository.URL, latest.URLs[0]),
		Repository: &repository,
	}, nil
}

func IsChartVersionNewer(next, current string) bool {
	return CompareChartVersions(next, current) > 0
}

func CompareChartVersions(a, b string) int {
	parsedA, errA := semver.ParseTolerant(a)
	parsedB, errB := semver.ParseTolerant(b)
	if errA == nil && errB == nil {
		return parsedA.Compare(parsedB)
	}
	if errA == nil {
		return 1
	}
	if errB == nil {
		return -1
	}
	return strings.Compare(a, b)
}
