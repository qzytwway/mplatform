package modelserving

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	modelTypeChat      = "chat"
	modelTypeEmbedding = "embedding"
	sourceTypeLocal    = "local"
	sourceTypeS3       = "s3"
	sourceTypePVC      = "pvc"
	sourceTypeDataset  = "dataset"

	maxModelNameLength = 100
)

var modelNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

type modelRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	SourceType  string `json:"sourceType"`
	Claim       string `json:"claim"`
	Dataset     string `json:"dataset"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

func (h *Handler) ListModels(c *gin.Context) {
	items := []model.ServedModel{}
	if err := model.DB.Order("name").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) CreateModel(c *gin.Context) {
	var req modelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	served, err := parseModelRequest(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !ensureModelNameFree(c, served.Name, 0) {
		return
	}
	if err := model.DB.Create(&served).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, served)
}

func (h *Handler) UpdateModel(c *gin.Context) {
	existing, ok := loadModelParam(c)
	if !ok {
		return
	}
	var req modelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	served, err := parseModelRequest(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !ensureModelNameFree(c, served.Name, existing.ID) {
		return
	}
	served.Model = existing.Model
	if err := model.DB.Save(&served).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, served)
}

func (h *Handler) DeleteModel(c *gin.Context) {
	existing, ok := loadModelParam(c)
	if !ok {
		return
	}
	if err := model.DB.Delete(existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "model deleted"})
}

func parseModelRequest(req *modelRequest) (model.ServedModel, error) {
	served := model.ServedModel{
		Name:        strings.TrimSpace(req.Name),
		Type:        req.Type,
		SourceType:  req.SourceType,
		Path:        strings.TrimSpace(req.Path),
		Description: strings.TrimSpace(req.Description),
	}
	if len(served.Name) > maxModelNameLength || !modelNamePattern.MatchString(served.Name) {
		return served, fmt.Errorf("name must start with a letter or digit, contain only letters, digits, '.', '_' or '-', and be at most %d characters", maxModelNameLength)
	}
	if served.Type != modelTypeChat && served.Type != modelTypeEmbedding {
		return served, fmt.Errorf("type must be %s or %s", modelTypeChat, modelTypeEmbedding)
	}
	switch served.SourceType {
	case sourceTypeLocal:
		if !path.IsAbs(served.Path) {
			return served, fmt.Errorf("path of a local model must be an absolute directory on the node")
		}
		served.Path = path.Clean(served.Path)
	case sourceTypeS3:
		served.Path = strings.Trim(served.Path, "/")
		if served.Path == "" {
			return served, fmt.Errorf("path of an s3 model must be a key prefix inside the bucket")
		}
	case sourceTypePVC:
		served.Claim = strings.TrimSpace(req.Claim)
		if errs := validation.IsDNS1123Subdomain(served.Claim); len(errs) > 0 {
			return served, fmt.Errorf("claim must be the name of a PersistentVolumeClaim: %s", strings.Join(errs, "; "))
		}
		if slices.Contains(strings.Split(served.Path, "/"), "..") {
			return served, fmt.Errorf("path of a pvc model must be a directory inside the volume")
		}
		// An empty sub-path mounts the whole volume.
		served.Path = strings.Trim(path.Clean("/"+served.Path), "/")
	case sourceTypeDataset:
		served.Dataset = strings.TrimSpace(req.Dataset)
		if _, _, ok := splitDataset(served.Dataset); !ok {
			return served, fmt.Errorf("dataset must be the <namespace>/<name> of a Fluid Dataset")
		}
		if slices.Contains(strings.Split(served.Path, "/"), "..") {
			return served, fmt.Errorf("path of a dataset model must be a directory inside the dataset")
		}
		served.Path = strings.Trim(path.Clean("/"+served.Path), "/")
	default:
		return served, fmt.Errorf("sourceType must be %s, %s, %s or %s", sourceTypeLocal, sourceTypeS3, sourceTypePVC, sourceTypeDataset)
	}
	return served, nil
}

// splitDataset returns the namespace and name of a <namespace>/<name>
// Dataset reference, and whether both are valid.
func splitDataset(dataset string) (namespace, name string, ok bool) {
	namespace, name, found := strings.Cut(dataset, "/")
	ok = found && len(validation.IsDNS1123Label(namespace)) == 0 && len(validation.IsDNS1123Subdomain(name)) == 0
	return namespace, name, ok
}

// ensureModelNameFree writes the error response and returns false when another
// model already uses name.
func ensureModelNameFree(c *gin.Context, name string, ownID uint) bool {
	var count int64
	if err := model.DB.Model(&model.ServedModel{}).Where("name = ? AND id <> ?", name, ownID).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("model %s already exists", name)})
		return false
	}
	return true
}

func loadModelParam(c *gin.Context) (*model.ServedModel, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model id"})
		return nil, false
	}
	served, err := loadModel(uint(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return nil, false
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	return served, true
}

func loadModel(id uint) (*model.ServedModel, error) {
	var served model.ServedModel
	if err := model.DB.First(&served, id).Error; err != nil {
		return nil, err
	}
	return &served, nil
}
