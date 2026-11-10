package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/helmutil"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
)

const (
	helmActionTimeout  = 5 * time.Minute
	defaultStorageSize = "20Gi"
	defaultRetention   = "7d"
	defaultNodePort    = 30428
	minNodePort        = 30000
	maxNodePort        = 32767
)

var (
	retentionPattern = regexp.MustCompile(`^[1-9][0-9]*[hdwy]$`)

	errAlreadyInstalled = errors.New("already installed")
)

type Handler struct {
	cm *cluster.ClusterManager
}

func NewHandler(cm *cluster.ClusterManager) *Handler {
	return &Handler{cm: cm}
}

func (h *Handler) RegisterAdminRoutes(group *gin.RouterGroup) {
	g := group.Group("/logging")
	g.GET("", h.GetStatus)
	g.POST("/storage", h.InstallStorage)
	g.DELETE("/storage", h.UninstallStorage)
	g.POST("/collectors/:cluster", h.EnableCollector)
	g.DELETE("/collectors/:cluster", h.DisableCollector)
}

type installStorageRequest struct {
	Cluster      string `json:"cluster" binding:"required"`
	StorageClass string `json:"storageClass"`
	StorageSize  string `json:"storageSize"`
	Retention    string `json:"retention"`
	NodeHost     string `json:"nodeHost" binding:"required"`
	NodePort     int    `json:"nodePort"`
}

type clusterCollectorStatus struct {
	Name       string `json:"name"`
	Collecting bool   `json:"collecting"`
	Error      string `json:"error,omitempty"`
}

func loadStorage() (*model.LogStorage, error) {
	var storage model.LogStorage
	if err := model.DB.First(&storage).Error; err != nil {
		return nil, err
	}
	return &storage, nil
}

func (h *Handler) GetStatus(c *gin.Context) {
	images, err := requiredImages()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	storage, err := loadStorage()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	clusters, err := h.collectorStatuses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"storage": storage, "images": images, "clusters": clusters})
}

func (h *Handler) collectorStatuses() ([]clusterCollectorStatus, error) {
	clusters, err := model.ListClusters()
	if err != nil {
		return nil, err
	}
	statuses := make([]clusterCollectorStatus, 0, len(clusters))
	for _, stored := range clusters {
		if !stored.Enable {
			continue
		}
		status := clusterCollectorStatus{Name: stored.Name}
		if cs, err := h.cm.GetClientSet(stored.Name); err != nil {
			status.Error = err.Error()
		} else if status.Collecting, err = releaseExists(cs, vectorRelease); err != nil {
			status.Error = err.Error()
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func releaseExists(cs *cluster.ClientSet, name string) (bool, error) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, Namespace)
	if err != nil {
		return false, err
	}
	if _, err := helmutil.GetRelease(cfg, name); err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (h *Handler) InstallStorage(c *gin.Context) {
	var req installStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	opts, writeURL, err := parseInstallStorageRequest(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := loadStorage(); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "log storage is already installed"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cs, err := h.cm.GetClientSet(req.Cluster)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	password, err := randomPassword()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	loadedChart, err := loadVictoriaLogsChart()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := installRelease(c, cs, victoriaLogsRelease, loadedChart, victoriaLogsValues(opts), password); err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}

	storage := model.LogStorage{
		ClusterName: cs.Name,
		Namespace:   Namespace,
		ServiceURL:  victoriaLogsServiceURL(),
		WriteURL:    writeURL,
		Username:    authUsername,
		Password:    model.SecretString(password),
	}
	if err := model.DB.Create(&storage).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, storage)
}

func parseInstallStorageRequest(req *installStorageRequest) (storageOptions, string, error) {
	opts := storageOptions{
		StorageClass: strings.TrimSpace(req.StorageClass),
		StorageSize:  strings.TrimSpace(req.StorageSize),
		Retention:    strings.TrimSpace(req.Retention),
		NodePort:     req.NodePort,
	}
	if opts.StorageSize == "" {
		opts.StorageSize = defaultStorageSize
	}
	if opts.Retention == "" {
		opts.Retention = defaultRetention
	}
	if opts.NodePort == 0 {
		opts.NodePort = defaultNodePort
	}
	if quantity, err := resource.ParseQuantity(opts.StorageSize); err != nil || quantity.Sign() <= 0 {
		return opts, "", fmt.Errorf("storage size must be a positive quantity such as 20Gi")
	}
	if opts.StorageClass != "" && len(validation.IsDNS1123Subdomain(opts.StorageClass)) > 0 {
		return opts, "", fmt.Errorf("storage class must be a valid resource name")
	}
	if !retentionPattern.MatchString(opts.Retention) {
		return opts, "", fmt.Errorf("retention must be a number followed by h, d, w or y, such as 7d")
	}
	if opts.NodePort < minNodePort || opts.NodePort > maxNodePort {
		return opts, "", fmt.Errorf("node port must be between %d and %d", minNodePort, maxNodePort)
	}
	host := strings.TrimSpace(req.NodeHost)
	if net.ParseIP(host) == nil && len(validation.IsDNS1123Subdomain(host)) > 0 {
		return opts, "", fmt.Errorf("node address must be an IP address or host name")
	}
	return opts, "http://" + net.JoinHostPort(host, strconv.Itoa(opts.NodePort)), nil
}

func (h *Handler) UninstallStorage(c *gin.Context) {
	storage, err := loadStorage()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "log storage is not installed"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	statuses, err := h.collectorStatuses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, status := range statuses {
		if status.Collecting {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("disable log collection on cluster %s first", status.Name)})
			return
		}
	}
	cs, err := h.cm.GetClientSet(storage.ClusterName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := uninstallRelease(c, cs, victoriaLogsRelease); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := deleteAuthSecret(c.Request.Context(), cs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := model.DB.Delete(storage).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "log storage uninstalled"})
}

func (h *Handler) EnableCollector(c *gin.Context) {
	storage, err := loadStorage()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "install log storage first"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// VL-Extra-Fields is a comma-separated list, so the name must not split it.
	if strings.Contains(cs.Name, ",") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster names containing commas cannot collect logs"})
		return
	}
	writeURL := storage.WriteURL
	if cs.Name == storage.ClusterName {
		writeURL = storage.ServiceURL
	}
	loadedChart, err := loadVectorChart()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	values := vectorValues(collectorOptions{ClusterName: cs.Name, WriteURL: writeURL})
	if err := installRelease(c, cs, vectorRelease, loadedChart, values, string(storage.Password)); err != nil {
		c.JSON(installErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "log collection enabled"})
}

func (h *Handler) DisableCollector(c *gin.Context) {
	cs, err := h.cm.GetClientSet(c.Param("cluster"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := uninstallRelease(c, cs, vectorRelease); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// The storage cluster's VictoriaLogs still reads the same Secret.
	storage, err := loadStorage()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if storage == nil || storage.ClusterName != cs.Name {
		if err := deleteAuthSecret(c.Request.Context(), cs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "log collection disabled"})
}

func installRelease(c *gin.Context, cs *cluster.ClientSet, name string, loadedChart *chart.Chart, values map[string]any, password string) (err error) {
	ctx := c.Request.Context()
	if err := ensureNamespace(ctx, cs); err != nil {
		return err
	}
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, Namespace)
	if err != nil {
		return err
	}
	existing, err := helmutil.GetRelease(cfg, name)
	switch {
	case err == nil && (existing.Info == nil || existing.Info.Status != releasecommon.StatusFailed):
		return fmt.Errorf("%s is %w", name, errAlreadyInstalled)
	case err == nil:
		// A failed earlier attempt still holds the release name.
		if err := uninstallRelease(c, cs, name); err != nil {
			return err
		}
	case !errors.Is(err, driver.ErrReleaseNotFound):
		return err
	}
	if err := ensureAuthSecret(ctx, cs, password); err != nil {
		return err
	}
	imageRegistry, err := helmutil.ClusterImageRegistry(cs.Name)
	if err != nil {
		return err
	}

	var rel *release.Release
	defer func() {
		recordHistory(c, cs, "install", name, nil, rel, err)
	}()
	rel, err = helmutil.InstallRelease(ctx, cfg, loadedChart, values, helmutil.InstallReleaseOptions{
		ReleaseName:   name,
		Namespace:     Namespace,
		Timeout:       helmActionTimeout,
		Description:   "Installed by MPlatform logging",
		ImageRegistry: imageRegistry,
	})
	if err != nil {
		// Remove the failed release so the install can be retried, e.g. with
		// another NodePort.
		if _, getErr := helmutil.GetRelease(cfg, name); getErr == nil {
			if uninstallErr := helmutil.UninstallRelease(cfg, name, helmutil.UninstallReleaseOptions{Timeout: helmActionTimeout}); uninstallErr != nil {
				klog.Warningf("Failed to remove failed release %s in cluster %s: %v", name, cs.Name, uninstallErr)
			}
		}
	}
	return err
}

func installErrorStatus(err error) int {
	if errors.Is(err, errAlreadyInstalled) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func uninstallRelease(c *gin.Context, cs *cluster.ClientSet, name string) (err error) {
	cfg, err := helmutil.NewActionConfig(cs.K8sClient.Configuration, Namespace)
	if err != nil {
		return err
	}
	current, err := helmutil.GetRelease(cfg, name)
	if err != nil {
		return err
	}
	defer func() {
		recordHistory(c, cs, "delete", name, current, nil, err)
	}()
	return helmutil.UninstallRelease(cfg, name, helmutil.UninstallReleaseOptions{
		Timeout:     helmActionTimeout,
		Description: "Uninstalled by MPlatform logging",
	})
}

func recordHistory(c *gin.Context, cs *cluster.ClientSet, opType, name string, prev, curr *release.Release, err error) {
	user := c.MustGet("user").(model.User)
	helmutil.RecordReleaseHistory(cs.Name, user.ID, "manual", opType, name, Namespace, prev, curr, err == nil, err)
}

// ensureNamespace creates the logging namespace with the privileged Pod
// Security level, since Vector mounts host paths to read container logs.
func ensureNamespace(ctx context.Context, cs *cluster.ClientSet) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   Namespace,
			Labels: map[string]string{"pod-security.kubernetes.io/enforce": "privileged"},
		},
	}
	_, err := cs.K8sClient.ClientSet.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		klog.V(2).Infof("Namespace %s already exists in cluster %s", Namespace, cs.Name)
		return nil
	}
	return err
}

func ensureAuthSecret(ctx context.Context, cs *cluster.ClientSet, password string) error {
	secrets := cs.K8sClient.ClientSet.CoreV1().Secrets(Namespace)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: authSecretName},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{authSecretKey: password},
	}
	_, err := secrets.Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to store log storage credentials: %w", err)
	}
	return nil
}

func deleteAuthSecret(ctx context.Context, cs *cluster.ClientSet) error {
	err := cs.K8sClient.ClientSet.CoreV1().Secrets(Namespace).Delete(ctx, authSecretName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete log storage credentials: %w", err)
	}
	return nil
}

func randomPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate password: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
