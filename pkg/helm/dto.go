package helm

import (
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
)

type createHelmRepositoryRequest struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type"`
	URL       string `json:"url" binding:"required"`
	PlainHTTP bool   `json:"plainHTTP"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

type helmRepositoryResponse struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	URL       string    `json:"url"`
	PlainHTTP bool      `json:"plainHTTP"`
	Username  string    `json:"username,omitempty"`
	HasAuth   bool      `json:"hasAuth"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type helmChart struct {
	RepositoryID   uint                `json:"repositoryId"`
	RepositoryName string              `json:"repositoryName"`
	RepositoryURL  string              `json:"repositoryUrl"`
	Source         string              `json:"source,omitempty"`
	Name           string              `json:"name"`
	Version        string              `json:"version"`
	AppVersion     string              `json:"appVersion,omitempty"`
	KubeVersion    string              `json:"kubeVersion,omitempty"`
	Description    string              `json:"description,omitempty"`
	Icon           string              `json:"icon,omitempty"`
	Home           string              `json:"home,omitempty"`
	ChartURL       string              `json:"chartUrl,omitempty"`
	Sources        []string            `json:"sources,omitempty"`
	Keywords       []string            `json:"keywords,omitempty"`
	Maintainers    []*chart.Maintainer `json:"maintainers,omitempty"`
	Deprecated     bool                `json:"deprecated,omitempty"`
	UpdatedAt      *time.Time          `json:"updatedAt,omitempty"`
}

type helmChartVersion struct {
	Version     string     `json:"version"`
	AppVersion  string     `json:"appVersion,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

type helmChartDetail struct {
	helmChart
	Readme   string             `json:"readme,omitempty"`
	Versions []helmChartVersion `json:"versions"`
}

type helmChartContentResponse struct {
	Content   string          `json:"content,omitempty"`
	Templates []chartTemplate `json:"templates,omitempty"`
}

type chartTemplate struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type helmChartContent struct {
	Readme    string
	Values    string
	Templates []chartTemplate
	Metadata  *chart.Metadata
}
