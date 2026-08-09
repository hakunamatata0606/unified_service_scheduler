package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
)

type Handler struct {
	queries *db.Queries
}

func NewRouter(queries *db.Queries) *gin.Engine {
	handler := &Handler{queries: queries}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := router.Group("/api/v1")
	v1.POST("/appointments", handler.createAppointment)
	v1.GET("/appointments/:appointmentId", handler.getAppointment)
	v1.GET("/appointments", handler.listAppointments)
	v1.GET("/availability", handler.getAvailability)

	return router
}

func (h *Handler) createAppointment(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "appointment booking is not implemented yet"})
}

func (h *Handler) getAppointment(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "appointment lookup is not implemented yet"})
}

func (h *Handler) listAppointments(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "appointment listing is not implemented yet"})
}

func (h *Handler) getAvailability(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "availability lookup is not implemented yet"})
}
