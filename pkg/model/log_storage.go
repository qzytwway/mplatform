package model

// LogStorage is the single central VictoriaLogs installation that every
// cluster's Vector agent writes to.
type LogStorage struct {
	Model
	ClusterName string `json:"clusterName" gorm:"type:varchar(100);not null"`
	Namespace   string `json:"namespace" gorm:"type:varchar(255);not null"`
	// ServiceURL is the in-cluster address used by the platform and by the
	// Vector agent running in the storage cluster.
	ServiceURL string `json:"serviceURL" gorm:"type:text;not null"`
	// WriteURL is the NodePort address Vector agents in other clusters use.
	WriteURL string       `json:"writeURL" gorm:"type:text;not null"`
	Username string       `json:"-" gorm:"type:varchar(255);not null"`
	Password SecretString `json:"-" gorm:"type:text"`
}
