package model

// HelmRepositoryTypeHarbor marks a repository whose URL is oci://<harbor>/<project>
// and whose charts are listed through the Harbor API.
const HelmRepositoryTypeHarbor = "harbor"

type HelmRepository struct {
	Model
	Name      string       `json:"name" gorm:"type:varchar(255);uniqueIndex;not null"`
	Type      string       `json:"type" gorm:"type:varchar(32);not null;default:''"`
	URL       string       `json:"url" gorm:"type:varchar(1024);not null"`
	PlainHTTP bool         `json:"plainHTTP" gorm:"not null;default:false"`
	Username  string       `json:"username,omitempty" gorm:"type:varchar(255)"`
	Password  SecretString `json:"-" gorm:"type:text"`
}
