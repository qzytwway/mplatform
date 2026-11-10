package model

// ModelGateway is the Higress gateway the platform installed in a cluster to
// expose published inference services on a NodePort.
type ModelGateway struct {
	Model
	ClusterName string `json:"clusterName" gorm:"type:varchar(100);uniqueIndex;not null"`
	NodeHost    string `json:"nodeHost" gorm:"type:varchar(255);not null"`
	NodePort    int    `json:"nodePort" gorm:"not null"`
	// Hub replaces the registry Higress pulls its images and plugins from.
	Hub string `json:"hub" gorm:"type:text"`
}

// ModelCaller is a consumer of a cluster's model gateway. Models lists the
// model names it may call unless AllowAll is set.
type ModelCaller struct {
	Model
	ClusterName string       `json:"-" gorm:"type:varchar(100);not null;uniqueIndex:idx_model_callers_cluster_name"`
	Name        string       `json:"name" gorm:"type:varchar(63);not null;uniqueIndex:idx_model_callers_cluster_name"`
	Description string       `json:"description" gorm:"type:text"`
	Enabled     bool         `json:"enabled" gorm:"type:boolean"`
	APIKey      SecretString `json:"-" gorm:"type:text"`
	AllowAll    bool         `json:"allowAll" gorm:"type:boolean"`
	Models      SliceString  `json:"models" gorm:"type:text"`
}
