package modelserving

import (
	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
)

type Handler struct {
	cm *cluster.ClusterManager
}

func NewHandler(cm *cluster.ClusterManager) *Handler {
	return &Handler{cm: cm}
}

func (h *Handler) RegisterAdminRoutes(group *gin.RouterGroup) {
	g := group.Group("/model-serving")
	g.GET("/storage", h.GetStorage)
	g.PUT("/storage", h.UpdateStorage)
	g.DELETE("/storage", h.DeleteStorage)

	g.GET("/models", h.ListModels)
	g.POST("/models", h.CreateModel)
	g.PUT("/models/:id", h.UpdateModel)
	g.DELETE("/models/:id", h.DeleteModel)

	g.GET("/clusters/:cluster/status", h.GetStatus)
	g.POST("/clusters/:cluster/lws", h.InstallLWS)
	g.DELETE("/clusters/:cluster/lws", h.UninstallLWS)
	g.GET("/clusters/:cluster/services", h.ListServices)
	g.POST("/clusters/:cluster/services", h.CreateService)
	g.GET("/clusters/:cluster/services/:namespace/:name", h.GetService)
	g.PUT("/clusters/:cluster/services/:namespace/:name/scale", h.ScaleService)
	g.DELETE("/clusters/:cluster/services/:namespace/:name", h.DeleteService)
	g.GET("/clusters/:cluster/gpus", h.ListGPUNodes)
}
