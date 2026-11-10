package model

// ModelStorage is the single S3-compatible object storage that inference
// services download offline models from.
type ModelStorage struct {
	Model
	Endpoint        string       `json:"endpoint" gorm:"type:text;not null"`
	Bucket          string       `json:"bucket" gorm:"type:varchar(255);not null"`
	Region          string       `json:"region" gorm:"type:varchar(100)"`
	AccessKey       string       `json:"accessKey" gorm:"type:varchar(255);not null"`
	SecretKey       SecretString `json:"-" gorm:"type:text"`
	DownloaderImage string       `json:"downloaderImage" gorm:"type:text;not null"`
}

// ServedModel is an offline model registered for inference services. Path is
// an absolute node directory for local models and a bucket key prefix for s3.
type ServedModel struct {
	Model
	Name        string `json:"name" gorm:"type:varchar(100);uniqueIndex;not null"`
	Type        string `json:"type" gorm:"type:varchar(20);not null"`
	SourceType  string `json:"sourceType" gorm:"type:varchar(20);not null"`
	Path        string `json:"path" gorm:"type:text;not null"`
	Description string `json:"description" gorm:"type:text"`
}
