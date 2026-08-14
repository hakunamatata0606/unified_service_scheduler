package httpapi

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/hakunamatata0606/unified_service_scheduler/internal/database"
	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
	"github.com/hakunamatata0606/unified_service_scheduler/internal/service"
)

//go:embed assets/*
var webAssets embed.FS

type appointmentService interface {
	CreateAppointment(context.Context, string, service.CreateAppointmentInput) (db.Appointment, error)
	GetAppointment(context.Context, pgtype.UUID) (db.Appointment, error)
	ListAppointments(context.Context, pgtype.UUID, pgtype.Timestamptz, pgtype.Timestamptz) ([]db.Appointment, error)
	GetAvailability(context.Context, pgtype.UUID, pgtype.UUID, pgtype.Timestamptz) (service.Availability, error)
	ListAppointmentDetails(context.Context, pgtype.UUID, pgtype.Timestamptz, pgtype.Timestamptz) ([]service.AppointmentDetail, error)
	ListCustomerAppointments(context.Context, pgtype.UUID) ([]service.AppointmentDetail, error)
	ListTechnicianAvailability(context.Context, pgtype.UUID, pgtype.UUID, pgtype.Timestamptz, pgtype.Timestamptz) ([]service.TechnicianAvailability, error)
	ListVehicles(context.Context, pgtype.UUID) ([]db.Vehicle, error)
	ListServiceTypes(context.Context) ([]db.ServiceType, error)
	ListDealerships(context.Context) ([]db.Dealership, error)
}

var _ appointmentService = (*service.AppointmentService)(nil)

type Handler struct {
	service appointmentService
	auth    authService
}

type authService interface {
	Login(context.Context, string, string) (database.User, error)
	GetUser(context.Context, pgtype.UUID) (database.User, error)
	ListVehicles(context.Context, pgtype.UUID) ([]db.Vehicle, error)
}

func NewRouter(appointmentService appointmentService, auth ...authService) *gin.Engine {
	var authentication authService
	if len(auth) > 0 {
		authentication = auth[0]
	}
	handler := &Handler{service: appointmentService, auth: authentication}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	assets, _ := fs.Sub(webAssets, "assets")
	router.StaticFS("/assets", http.FS(assets))
	router.GET("/", func(c *gin.Context) {
		index, err := webAssets.ReadFile("assets/index.html")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
	router.GET("/admin", func(c *gin.Context) {
		admin, err := webAssets.ReadFile("assets/admin.html")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", admin)
	})
	router.POST("/api/auth/login", handler.login)
	router.POST("/api/auth/logout", handler.logout)
	router.GET("/api/auth/me", handler.me)

	v1 := router.Group("/api/v1")
	v1.POST("/appointments", handler.createAppointment)
	v1.GET("/appointments/:appointmentId", handler.getAppointment)
	v1.GET("/appointments", handler.listAppointments)
	v1.GET("/my/appointments", handler.listCustomerAppointments)
	v1.GET("/availability", handler.getAvailability)
	v1.GET("/admin/appointments", handler.listAppointmentDetails)
	v1.GET("/admin/technicians", handler.listTechnicianAvailability)
	v1.GET("/vehicles", handler.listVehicles)
	v1.GET("/service-types", handler.listServiceTypes)
	v1.GET("/dealerships", handler.listDealerships)

	return router
}

func (h *Handler) listCustomerAppointments(c *gin.Context) {
	if h.auth == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	user, ok := h.currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	appointments, err := h.service.ListCustomerAppointments(c.Request.Context(), user.CustomerID)
	if err != nil {
		slog.Error("failed to list customer appointments", "customer_id", user.CustomerID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, appointments)
}

func (h *Handler) listAppointmentDetails(c *gin.Context) {
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
	appointments, err := h.service.ListAppointmentDetails(c.Request.Context(), dealershipID, start, end)
	if err != nil {
		slog.Error("failed to list appointment details", "dealership_id", dealershipID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, appointments)
}

func (h *Handler) listTechnicianAvailability(c *gin.Context) {
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
	start, end, err := parseInterval(c.Query("from"), c.Query("to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	technicians, err := h.service.ListTechnicianAvailability(c.Request.Context(), dealershipID, serviceTypeID, start, end)
	if err != nil {
		slog.Error("failed to list technician availability", "dealership_id", dealershipID, "service_type_id", serviceTypeID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, technicians)
}

func (h *Handler) listVehicles(c *gin.Context) {
	if h.auth != nil {
		user, ok := h.currentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		vehicles, err := h.auth.ListVehicles(c.Request.Context(), user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		c.JSON(http.StatusOK, vehicles)
		return
	}
	customerID, err := parseID(c.Query("customerId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid customerId"})
		return
	}
	vehicles, err := h.service.ListVehicles(c.Request.Context(), customerID)
	if err != nil {
		slog.Error("failed to list vehicles", "customer_id", customerID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, vehicles)
}

func (h *Handler) listServiceTypes(c *gin.Context) {
	serviceTypes, err := h.service.ListServiceTypes(c.Request.Context())
	if err != nil {
		slog.Error("failed to list service types", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, serviceTypes)
}

func (h *Handler) listDealerships(c *gin.Context) {
	dealerships, err := h.service.ListDealerships(c.Request.Context())
	if err != nil {
		slog.Error("failed to list dealerships", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, dealerships)
}

func (h *Handler) createAppointment(c *gin.Context) {
	var request createAppointmentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid appointment request"})
		return
	}

	vehicleID, err := parseID(request.VehicleID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid appointment request"})
		return
	}
	dealershipID, err := parseID(request.DealershipID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid appointment request"})
		return
	}
	serviceTypeID, err := parseID(request.ServiceTypeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid appointment request"})
		return
	}
	var customerID pgtype.UUID
	if h.auth != nil {
		user, ok := h.currentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		customerID = user.CustomerID
		vehicles, err := h.auth.ListVehicles(c.Request.Context(), user.ID)
		if err != nil || !containsVehicle(vehicles, vehicleID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "vehicle does not belong to current user"})
			return
		}
	} else {
		customerID, err = parseID(request.CustomerID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid appointment request"})
			return
		}
	}
	start, err := parseTimestamp(request.Start())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start or requestedStart"})
		return
	}

	appointment, err := h.service.CreateAppointment(c.Request.Context(), c.GetHeader("Idempotency-Key"), service.CreateAppointmentInput{
		CustomerID:    customerID,
		VehicleID:     vehicleID,
		DealershipID:  dealershipID,
		ServiceTypeID: serviceTypeID,
		StartTimeUtc:  start,
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
		case errors.Is(err, service.ErrVehicleNotOwned):
			c.JSON(http.StatusBadRequest, gin.H{"error": "vehicle does not belong to current user"})
		default:
			slog.Error("failed to create appointment", "dealership_id", dealershipID, "service_type_id", serviceTypeID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}
	c.JSON(http.StatusCreated, appointment)
}

func (h *Handler) login(c *gin.Context) {
	if h.auth == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication is not configured"})
		return
	}
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login request"})
		return
	}
	user, err := h.auth.Login(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}
	cookie := &http.Cookie{Name: "session_user", Value: user.ID.String(), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	http.SetCookie(c.Writer, cookie)
	c.JSON(http.StatusOK, gin.H{"id": user.ID, "email": user.Email})
}

func (h *Handler) logout(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "session_user", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	c.Status(http.StatusNoContent)
}

func (h *Handler) me(c *gin.Context) {
	user, ok := h.currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": user.ID, "email": user.Email, "customer_id": user.CustomerID})
}

func (h *Handler) currentUser(c *gin.Context) (database.User, bool) {
	if h.auth == nil {
		return database.User{}, false
	}
	var id pgtype.UUID
	if err := id.Scan(c.Request.Header.Get("X-User-ID")); err == nil && c.Request.Header.Get("X-User-ID") != "" {
		user, err := h.auth.GetUser(c.Request.Context(), id)
		return user, err == nil
	}
	cookie, err := c.Request.Cookie("session_user")
	if err != nil || id.Scan(cookie.Value) != nil {
		return database.User{}, false
	}
	user, err := h.auth.GetUser(c.Request.Context(), id)
	return user, err == nil
}

func containsVehicle(vehicles []db.Vehicle, id pgtype.UUID) bool {
	for _, vehicle := range vehicles {
		if vehicle.ID == id {
			return true
		}
	}
	return false
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
		if h.auth != nil {
			user, ok := h.currentUser(c)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
				return
			}
			if appointment.CustomerID != user.CustomerID {
				c.JSON(http.StatusNotFound, gin.H{"error": "appointment not found"})
				return
			}
		}
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
	if h.auth != nil {
		user, ok := h.currentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		owned := appointments[:0]
		for _, appointment := range appointments {
			if appointment.CustomerID == user.CustomerID {
				owned = append(owned, appointment)
			}
		}
		appointments = owned
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
	start, err := parseTimestamp(c.Query("start"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start"})
		return
	}
	availability, err := h.service.GetAvailability(c.Request.Context(), dealershipID, serviceTypeID, start)
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
	RequestedStart string `json:"requestedStart"`
	StartTimeUtc   string `json:"startTimeUtc"`
}

func (r createAppointmentRequest) Start() string {
	if r.RequestedStart != "" {
		return r.RequestedStart
	}
	return r.StartTimeUtc
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

func parseTimestamp(value string) (pgtype.Timestamptz, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return pgtype.Timestamptz{}, err
	}
	return pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}, nil
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
