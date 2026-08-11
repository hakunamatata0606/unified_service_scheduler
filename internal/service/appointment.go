package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
)

var ErrNoAvailability = errors.New("no appointment availability")
var ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
var ErrIdempotencyConflict = errors.New("idempotency key was reused with a different request")
var ErrIdempotencyInProgress = errors.New("idempotency request is already in progress")

type AppointmentStore interface {
	CreateAppointment(context.Context, db.CreateAppointmentParams) (db.Appointment, error)
	FindAvailableServiceBays(context.Context, db.FindAvailableServiceBaysParams) ([]db.ServiceBay, error)
	FindAvailableTechnicians(context.Context, db.FindAvailableTechniciansParams) ([]db.Technician, error)
	GetAppointment(context.Context, pgtype.UUID) (db.Appointment, error)
	CreateIdempotencyRequest(context.Context, db.CreateIdempotencyRequestParams) (int64, error)
	GetIdempotencyRequest(context.Context, string) (db.IdempotencyRequest, error)
	CompleteIdempotencyRequest(context.Context, db.CompleteIdempotencyRequestParams) (db.IdempotencyRequest, error)
	ListAppointmentsByDealership(context.Context, db.ListAppointmentsByDealershipParams) ([]db.Appointment, error)
}

type AppointmentService struct {
	store AppointmentStore
}

func NewAppointmentService(store AppointmentStore) *AppointmentService {
	return &AppointmentService{store: store}
}

type CreateAppointmentInput struct {
	CustomerID    pgtype.UUID
	VehicleID     pgtype.UUID
	DealershipID  pgtype.UUID
	ServiceTypeID pgtype.UUID
	TechnicianID  pgtype.UUID
	ServiceBayID  pgtype.UUID
	StartTimeUtc  pgtype.Timestamptz
	EndTimeUtc    pgtype.Timestamptz
}

func (s *AppointmentService) CreateAppointment(ctx context.Context, key string, input CreateAppointmentInput) (db.Appointment, error) {
	if key == "" {
		return db.Appointment{}, ErrIdempotencyKeyRequired
	}
	requestHash, err := hashCreateAppointmentInput(input)
	if err != nil {
		return db.Appointment{}, err
	}
	claimed, err := s.store.CreateIdempotencyRequest(ctx, db.CreateIdempotencyRequestParams{
		Key:          key,
		RequestHash:  requestHash,
		ExpiresAtUtc: pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true},
	})
	if err != nil {
		return db.Appointment{}, err
	}
	if claimed == 0 {
		existing, err := s.store.GetIdempotencyRequest(ctx, key)
		if err != nil {
			return db.Appointment{}, err
		}
		if existing.RequestHash != requestHash {
			return db.Appointment{}, ErrIdempotencyConflict
		}
		if existing.Status == "completed" && existing.AppointmentID.Valid {
			return s.store.GetAppointment(ctx, existing.AppointmentID)
		}
		return db.Appointment{}, ErrIdempotencyInProgress
	}

	technicianID := input.TechnicianID
	serviceBayID := input.ServiceBayID
	if !technicianID.Valid || !serviceBayID.Valid {
		technicians, err := s.store.FindAvailableTechnicians(ctx, db.FindAvailableTechniciansParams{
			ServiceTypeID:  input.ServiceTypeID,
			DealershipID:   input.DealershipID,
			RequestedEnd:   input.EndTimeUtc,
			RequestedStart: input.StartTimeUtc,
		})
		if err != nil {
			return db.Appointment{}, err
		}
		bays, err := s.store.FindAvailableServiceBays(ctx, db.FindAvailableServiceBaysParams{
			ServiceTypeID:  input.ServiceTypeID,
			DealershipID:   input.DealershipID,
			RequestedEnd:   input.EndTimeUtc,
			RequestedStart: input.StartTimeUtc,
		})
		if err != nil {
			return db.Appointment{}, err
		}
		if len(technicians) == 0 || len(bays) == 0 {
			return db.Appointment{}, ErrNoAvailability
		}
		if !technicianID.Valid {
			technicianID = technicians[0].ID
		}
		if !serviceBayID.Valid {
			serviceBayID = bays[0].ID
		}
	}

	appointmentID, err := newUUID()
	if err != nil {
		return db.Appointment{}, err
	}
	appointment, err := s.store.CreateAppointment(ctx, db.CreateAppointmentParams{
		ID:            appointmentID,
		CustomerID:    input.CustomerID,
		VehicleID:     input.VehicleID,
		DealershipID:  input.DealershipID,
		TechnicianID:  technicianID,
		ServiceBayID:  serviceBayID,
		ServiceTypeID: input.ServiceTypeID,
		StartTimeUtc:  input.StartTimeUtc,
		EndTimeUtc:    input.EndTimeUtc,
	})
	if err != nil {
		return db.Appointment{}, err
	}
	responseBody, err := json.Marshal(appointment)
	if err != nil {
		return db.Appointment{}, err
	}
	if _, err := s.store.CompleteIdempotencyRequest(ctx, db.CompleteIdempotencyRequestParams{
		AppointmentID:  appointment.ID,
		ResponseStatus: pgtype.Int4{Int32: 201, Valid: true},
		ResponseBody:   responseBody,
		Key:            key,
	}); err != nil {
		return db.Appointment{}, err
	}
	return appointment, nil
}

func hashCreateAppointmentInput(input CreateAppointmentInput) (string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}

func (s *AppointmentService) GetAppointment(ctx context.Context, id pgtype.UUID) (db.Appointment, error) {
	return s.store.GetAppointment(ctx, id)
}

func (s *AppointmentService) ListAppointments(ctx context.Context, dealershipID pgtype.UUID, start, end pgtype.Timestamptz) ([]db.Appointment, error) {
	return s.store.ListAppointmentsByDealership(ctx, db.ListAppointmentsByDealershipParams{
		DealershipID: dealershipID,
		RangeEnd:     end,
		RangeStart:   start,
	})
}

type Availability struct {
	Available       bool            `json:"available"`
	DurationMinutes int             `json:"durationMinutes"`
	Technicians     []db.Technician `json:"technicians"`
	ServiceBays     []db.ServiceBay `json:"serviceBays"`
}

func (s *AppointmentService) GetAvailability(ctx context.Context, dealershipID, serviceTypeID pgtype.UUID, start, end pgtype.Timestamptz) (Availability, error) {
	technicians, err := s.store.FindAvailableTechnicians(ctx, db.FindAvailableTechniciansParams{
		ServiceTypeID:  serviceTypeID,
		DealershipID:   dealershipID,
		RequestedEnd:   end,
		RequestedStart: start,
	})
	if err != nil {
		return Availability{}, err
	}
	bays, err := s.store.FindAvailableServiceBays(ctx, db.FindAvailableServiceBaysParams{
		ServiceTypeID:  serviceTypeID,
		DealershipID:   dealershipID,
		RequestedEnd:   end,
		RequestedStart: start,
	})
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Available:       len(technicians) > 0 && len(bays) > 0,
		DurationMinutes: int(end.Time.Sub(start.Time).Minutes()),
		Technicians:     technicians,
		ServiceBays:     bays,
	}, nil
}

func newUUID() (pgtype.UUID, error) {
	var id pgtype.UUID
	if _, err := rand.Read(id.Bytes[:]); err != nil {
		return pgtype.UUID{}, err
	}
	id.Valid = true
	return id, nil
}
