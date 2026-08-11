package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
	"github.com/hakunamatata0606/unified_service_scheduler/internal/service"
)

type appointmentService interface {
	CreateAppointment(context.Context, string, service.CreateAppointmentInput) (db.Appointment, error)
	GetAppointment(context.Context, pgtype.UUID) (db.Appointment, error)
	ListAppointments(context.Context, pgtype.UUID, pgtype.Timestamptz, pgtype.Timestamptz) ([]db.Appointment, error)
	GetAvailability(context.Context, pgtype.UUID, pgtype.UUID, pgtype.Timestamptz, pgtype.Timestamptz) (service.Availability, error)
}

var _ appointmentService = (*service.AppointmentService)(nil)

type Handler struct {
	service appointmentService
}

func NewRouter(appointmentService appointmentService) *gin.Engine {
	handler := &Handler{service: appointmentService}

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
	var request createAppointmentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid appointment request"})
		return
	}

	customerID, vehicleID, dealershipID, serviceTypeID, err := parseRequiredIDs(
		request.CustomerID,
		request.VehicleID,
		request.DealershipID,
		request.ServiceTypeID,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	start, end, err := parseInterval(request.Start(), request.End())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	technicianID, err := parseOptionalID(request.TechnicianID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid technicianId"})
		return
	}
	serviceBayID, err := parseOptionalID(request.ServiceBayID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid serviceBayId"})
		return
	}

	appointment, err := h.service.CreateAppointment(c.Request.Context(), c.GetHeader("Idempotency-Key"), service.CreateAppointmentInput{
		CustomerID:    customerID,
		VehicleID:     vehicleID,
		DealershipID:  dealershipID,
		ServiceTypeID: serviceTypeID,
		TechnicianID:  technicianID,
		ServiceBayID:  serviceBayID,
		StartTimeUtc:  start,
		EndTimeUtc:    end,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrIdempotencyKeyRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key header is required"})
		case errors.Is(err, service.ErrIdempotencyConflict):
			c.JSON(http.StatusConflict, gin.H{"error": "idempotency key was reused with a different request"})
		case errors.Is(err, service.ErrIdempotencyInProgress):
			c.JSON(http.StatusConflict, gin.H{"error": "idempotency request is already in progress"})
		case errors.Is(err, service.ErrNoAvailability):
			c.JSON(http.StatusConflict, gin.H{"error": "no appointment availability"})
		default:
			slog.Error("failed to create appointment", "dealership_id", dealershipID, "service_type_id", serviceTypeID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}
	c.JSON(http.StatusCreated, appointment)
}

func (h *Handler) getAppointment(c *gin.Context) {
	appointmentID := c.Param("appointmentId")
	id, err := parseID(appointmentID)
	if err != nil {
		slog.Error("invalid appointment id", "appointment_id", appointmentID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid appointment id"})
		return
	}
	appointment, err := h.service.GetAppointment(c.Request.Context(), id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		slog.Error("appointment not found", "appointment_id", id, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "appointment not found"})
	case err != nil:
		slog.Error("failed to get appointment", "appointment_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	default:
		c.JSON(http.StatusOK, appointment)
	}
}

func (h *Handler) listAppointments(c *gin.Context) {
	dealershipID, err := parseID(c.Query("dealershipId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dealershipId"})
		return
	}
	start, end, err := parseInterval(c.Query("from"), c.Query("to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	appointments, err := h.service.ListAppointments(c.Request.Context(), dealershipID, start, end)
	if err != nil {
		slog.Error("failed to list appointments", "dealership_id", dealershipID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, appointments)
}

func (h *Handler) getAvailability(c *gin.Context) {
	dealershipID, err := parseID(c.Query("dealershipId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dealershipId"})
		return
	}
	serviceTypeID, err := parseID(c.Query("serviceTypeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid serviceTypeId"})
		return
	}
	start, end, err := parseInterval(c.Query("start"), c.Query("end"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	availability, err := h.service.GetAvailability(c.Request.Context(), dealershipID, serviceTypeID, start, end)
	if err != nil {
		slog.Error("failed to get availability", "dealership_id", dealershipID, "service_type_id", serviceTypeID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, availability)
}

type createAppointmentRequest struct {
	CustomerID     string `json:"customerId"`
	VehicleID      string `json:"vehicleId"`
	DealershipID   string `json:"dealershipId"`
	ServiceTypeID  string `json:"serviceTypeId"`
	TechnicianID   string `json:"technicianId"`
	ServiceBayID   string `json:"serviceBayId"`
	RequestedStart string `json:"requestedStart"`
	RequestedEnd   string `json:"requestedEnd"`
	StartTimeUtc   string `json:"startTimeUtc"`
	EndTimeUtc     string `json:"endTimeUtc"`
}

func (r createAppointmentRequest) Start() string {
	if r.RequestedStart != "" {
		return r.RequestedStart
	}
	return r.StartTimeUtc
}

func (r createAppointmentRequest) End() string {
	if r.RequestedEnd != "" {
		return r.RequestedEnd
	}
	return r.EndTimeUtc
}

func parseRequiredIDs(values ...string) (pgtype.UUID, pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	ids := make([]pgtype.UUID, len(values))
	for i, value := range values {
		id, err := parseID(value)
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, errors.New("invalid appointment request")
		}
		ids[i] = id
	}
	return ids[0], ids[1], ids[2], ids[3], nil
}

func parseOptionalID(value string) (pgtype.UUID, error) {
	if value == "" {
		return pgtype.UUID{}, nil
	}
	return parseID(value)
}

func parseID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func parseInterval(startValue, endValue string) (pgtype.Timestamptz, pgtype.Timestamptz, error) {
	start, err := time.Parse(time.RFC3339, startValue)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.Timestamptz{}, errors.New("invalid start or requestedStart")
	}
	end, err := time.Parse(time.RFC3339, endValue)
	if err != nil || !end.After(start) {
		return pgtype.Timestamptz{}, pgtype.Timestamptz{}, errors.New("invalid end or requestedEnd")
	}
	return pgtype.Timestamptz{Time: start.UTC(), Valid: true}, pgtype.Timestamptz{Time: end.UTC(), Valid: true}, nil
}
