package modelserving

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/distribution/reference"
	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
)

const defaultDownloaderImage = "amazon/aws-cli:latest"

type updateStorageRequest struct {
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	AccessKey       string `json:"accessKey"`
	SecretKey       string `json:"secretKey"`
	DownloaderImage string `json:"downloaderImage"`
}

func loadStorage() (*model.ModelStorage, error) {
	var storage model.ModelStorage
	if err := model.DB.First(&storage).Error; err != nil {
		return nil, err
	}
	return &storage, nil
}

func (h *Handler) GetStorage(c *gin.Context) {
	storage, err := loadStorage()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"storage": storage})
}

func (h *Handler) UpdateStorage(c *gin.Context) {
	var req updateStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	storage, err := parseStorageRequest(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing, err := loadStorage()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		storage.Model = existing.Model
		if storage.SecretKey == "" {
			storage.SecretKey = existing.SecretKey
		}
	}
	if storage.SecretKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "secret key is required"})
		return
	}
	if err := model.DB.Save(&storage).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, storage)
}

func parseStorageRequest(req *updateStorageRequest) (model.ModelStorage, error) {
	storage := model.ModelStorage{
		Endpoint:        strings.TrimRight(strings.TrimSpace(req.Endpoint), "/"),
		Bucket:          strings.TrimSpace(req.Bucket),
		Region:          strings.TrimSpace(req.Region),
		AccessKey:       strings.TrimSpace(req.AccessKey),
		SecretKey:       model.SecretString(req.SecretKey),
		DownloaderImage: strings.TrimSpace(req.DownloaderImage),
	}
	if storage.DownloaderImage == "" {
		storage.DownloaderImage = defaultDownloaderImage
	}
	endpoint, err := url.Parse(storage.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return storage, fmt.Errorf("endpoint must be an http or https URL such as http://minio.example:9000")
	}
	if storage.Bucket == "" || strings.ContainsAny(storage.Bucket, "/ \t") {
		return storage, fmt.Errorf("bucket must be a bucket name without slashes or spaces")
	}
	if storage.AccessKey == "" {
		return storage, fmt.Errorf("access key is required")
	}
	if _, err := reference.ParseNormalizedNamed(storage.DownloaderImage); err != nil {
		return storage, fmt.Errorf("downloader image must be a valid image reference")
	}
	return storage, nil
}

func (h *Handler) DeleteStorage(c *gin.Context) {
	storage, err := loadStorage()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model storage is not configured"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := model.DB.Delete(storage).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "model storage deleted"})
}
