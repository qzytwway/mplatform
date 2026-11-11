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
	// QuotaAdminKey authenticates the platform to the gateway's quota
	// administration path. It is empty until the gateway has Redis, i.e. for
	// a gateway installed before quotas existed and not upgraded yet.
	QuotaAdminKey SecretString `json:"-" gorm:"type:text"`
	// RedisPassword is empty for a gateway whose Redis was installed without
	// one and not upgraded yet.
	RedisPassword SecretString `json:"-" gorm:"type:text"`
}

// ModelCaller is a consumer of a cluster's model gateway. Models lists the
// model names it may call unless AllowAll is set. TokensPerMinute is its rate
// limit, 0 for none; the quota left to a QuotaLimited caller lives in the
// gateway's Redis.
type ModelCaller struct {
	Model
	ClusterName string       `json:"-" gorm:"type:varchar(100);not null;uniqueIndex:idx_model_callers_cluster_name"`
	Name        string       `json:"name" gorm:"type:varchar(63);not null;uniqueIndex:idx_model_callers_cluster_name"`
	Description string       `json:"description" gorm:"type:text"`
	Enabled     bool         `json:"enabled" gorm:"type:boolean"`
	APIKey      SecretString `json:"-" gorm:"type:text"`
	AllowAll    bool         `json:"allowAll" gorm:"type:boolean"`
	Models      SliceString  `json:"models" gorm:"type:text"`

	TokensPerMinute int64 `json:"tokensPerMinute" gorm:"not null;default:0"`
	QuotaLimited    bool  `json:"quotaLimited" gorm:"type:boolean"`
}
