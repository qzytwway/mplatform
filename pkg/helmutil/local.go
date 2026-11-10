package helmutil

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	repo "helm.sh/helm/v4/pkg/repo/v1"
	"k8s.io/klog/v2"
)

// LocalRepositoryName names the built-in repository backed by common.ChartDir.
const LocalRepositoryName = "local"

var ErrLocalChartNotFound = errors.New("chart is not in the local chart directory")

// EnsureLocalRepository creates the chart directory and registers it as the
// built-in local repository.
func EnsureLocalRepository() error {
	dir, err := filepath.Abs(common.ChartDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create chart directory %s: %w", dir, err)
	}
	repositoryURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(dir)}).String()

	var repository model.HelmRepository
	err = model.DB.Where("name = ?", LocalRepositoryName).First(&repository).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		repository = model.HelmRepository{Name: LocalRepositoryName, Type: model.HelmRepositoryTypeLocal, URL: repositoryURL}
		return model.DB.Create(&repository).Error
	case err != nil:
		return err
	case repository.Type != model.HelmRepositoryTypeLocal:
		return fmt.Errorf("repository name %q is already used by another repository", LocalRepositoryName)
	case repository.URL != repositoryURL:
		return model.DB.Model(&repository).Update("url", repositoryURL).Error
	}
	return nil
}

func LocalRepository() (model.HelmRepository, error) {
	var repository model.HelmRepository
	if err := model.DB.Where("type = ?", model.HelmRepositoryTypeLocal).First(&repository).Error; err != nil {
		return repository, fmt.Errorf("failed to load the local chart repository: %w", err)
	}
	return repository, nil
}

func localRepositoryDir(repository model.HelmRepository) string {
	parsed, err := url.Parse(repository.URL)
	if err != nil {
		return ""
	}
	return filepath.FromSlash(parsed.Path)
}

// loadLocalRepositoryIndex indexes the chart packages in the local directory.
// Invalid files are skipped so that one bad file does not hide the others.
func loadLocalRepositoryIndex(repository model.HelmRepository) (*repo.IndexFile, error) {
	files, err := filepath.Glob(filepath.Join(localRepositoryDir(repository), "*.tgz"))
	if err != nil {
		return nil, err
	}
	index := repo.NewIndexFile()
	for _, file := range files {
		loadedChart, err := loader.LoadFile(file)
		if err != nil {
			klog.Warningf("Skipping invalid chart package %s: %v", file, err)
			continue
		}
		if err := index.MustAdd(loadedChart.Metadata, filepath.Base(file), "", ""); err != nil {
			klog.Warningf("Skipping chart package %s: %v", file, err)
			continue
		}
		if info, err := os.Stat(file); err == nil {
			versions := index.Entries[loadedChart.Metadata.Name]
			versions[len(versions)-1].Created = info.ModTime()
		}
	}
	index.SortEntries()
	return index, nil
}

// loadLocalArchive loads a chart package, accepting only files directly inside
// the local repository directory.
func loadLocalArchive(chartPath string, repository *model.HelmRepository) (*chart.Chart, error) {
	if repository == nil || repository.Type != model.HelmRepositoryTypeLocal {
		return nil, fmt.Errorf("unsupported chartUrl scheme")
	}
	file := filepath.FromSlash(chartPath)
	if filepath.Dir(file) != filepath.Clean(localRepositoryDir(*repository)) || filepath.Ext(file) != ".tgz" {
		return nil, fmt.Errorf("chart package must be in the local chart directory")
	}
	return loader.LoadFile(file)
}

// LoadLocalChart loads the newest version of a chart in the local repository.
func LoadLocalChart(name string) (*chart.Chart, error) {
	repository, err := LocalRepository()
	if err != nil {
		return nil, err
	}
	index, err := loadLocalRepositoryIndex(repository)
	if err != nil {
		return nil, err
	}
	versions := index.Entries[name]
	if len(versions) == 0 {
		return nil, fmt.Errorf("%s: %w", name, ErrLocalChartNotFound)
	}
	return loader.LoadFile(filepath.Join(localRepositoryDir(repository), versions[0].URLs[0]))
}

// SaveLocalChart validates a chart package and stores it in the local
// repository as <name>-<version>.tgz, replacing an existing copy.
func SaveLocalChart(archive []byte) (*chart.Metadata, error) {
	loadedChart, err := loader.LoadArchive(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("not a valid chart package: %w", err)
	}
	metadata := loadedChart.Metadata
	fileName := metadata.Name + "-" + metadata.Version + ".tgz"
	if filepath.Base(fileName) != fileName || strings.HasPrefix(fileName, ".") {
		return nil, fmt.Errorf("invalid chart name %q", metadata.Name)
	}
	repository, err := LocalRepository()
	if err != nil {
		return nil, err
	}
	dir := localRepositoryDir(repository)

	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to store chart package: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(archive); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("failed to store chart package: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("failed to store chart package: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, fileName)); err != nil {
		return nil, fmt.Errorf("failed to store chart package: %w", err)
	}
	return metadata, nil
}
