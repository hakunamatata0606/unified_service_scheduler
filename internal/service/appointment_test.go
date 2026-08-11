package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
)

type appointmentStoreStub struct {
	appointment  db.Appointment
	technicians  []db.Technician
	serviceBays  []db.ServiceBay
	createParams db.CreateAppointmentParams
	createCalled bool
	err          error
	idempotency  db.IdempotencyRequest
	claimRows    int64
	claimSet     bool
}

func (s *appointmentStoreStub) CreateAppointment(_ context.Context, params db.CreateAppointmentParams) (db.Appointment, error) {
	s.createCalled = true
	s.createParams = params
	return s.appointment, s.err
}

func (s *appointmentStoreStub) FindAvailableServiceBays(context.Context, db.FindAvailableServiceBaysParams) ([]db.ServiceBay, error) {
	return s.serviceBays, s.err
}

func (s *appointmentStoreStub) FindAvailableTechnicians(context.Context, db.FindAvailableTechniciansParams) ([]db.Technician, error) {
	return s.technicians, s.err
}

func (s *appointmentStoreStub) CreateIdempotencyRequest(context.Context, db.CreateIdempotencyRequestParams) (int64, error) {
	if !s.claimSet {
		return 1, s.err
	}
	return s.claimRows, s.err
}

func (s *appointmentStoreStub) GetIdempotencyRequest(context.Context, string) (db.IdempotencyRequest, error) {
	return s.idempotency, s.err
}

func (s *appointmentStoreStub) CompleteIdempotencyRequest(_ context.Context, params db.CompleteIdempotencyRequestParams) (db.IdempotencyRequest, error) {
	s.idempotency = db.IdempotencyRequest{Status: "completed", AppointmentID: params.AppointmentID}
	return s.idempotency, s.err
}

func (s *appointmentStoreStub) GetAppointment(context.Context, pgtype.UUID) (db.Appointment, error) {
	return s.appointment, s.err
}

func (s *appointmentStoreStub) ListAppointmentsByDealership(context.Context, db.ListAppointmentsByDealershipParams) ([]db.Appointment, error) {
	return nil, s.err
}

func TestCreateAppointmentSelectsAvailableResources(t *testing.T) {
	technicianID := testUUID(1)
	serviceBayID := testUUID(2)
	store := &appointmentStoreStub{
		technicians: []db.Technician{{ID: technicianID}},
		serviceBays: []db.ServiceBay{{ID: serviceBayID}},
	}
	appointmentService := NewAppointmentService(store)

	appointment, err := appointmentService.CreateAppointment(context.Background(), "booking-1", CreateAppointmentInput{
		CustomerID:    testUUID(3),
		VehicleID:     testUUID(4),
		DealershipID:  testUUID(5),
		ServiceTypeID: testUUID(6),
		StartTimeUtc:  testTime(9),
		EndTimeUtc:    testTime(10),
	})
	if err != nil {
		t.Fatalf("CreateAppointment returned error: %v", err)
	}
	if !store.createCalled {
		t.Fatal("expected appointment to be created")
	}
	if store.createParams.TechnicianID != technicianID || store.createParams.ServiceBayID != serviceBayID {
		t.Fatalf("expected available resources to be selected, got %+v", store.createParams)
	}
	if !store.createParams.ID.Valid {
		t.Fatal("expected generated appointment id")
	}
	_ = appointment
}

func TestCreateAppointmentReturnsNoAvailability(t *testing.T) {
	store := &appointmentStoreStub{}
	appointmentService := NewAppointmentService(store)

	_, err := appointmentService.CreateAppointment(context.Background(), "booking-1", CreateAppointmentInput{
		CustomerID:    testUUID(3),
		VehicleID:     testUUID(4),
		DealershipID:  testUUID(5),
		ServiceTypeID: testUUID(6),
		StartTimeUtc:  testTime(9),
		EndTimeUtc:    testTime(10),
	})
	if err != ErrNoAvailability {
		t.Fatalf("expected ErrNoAvailability, got %v", err)
	}
}

func TestCreateAppointmentReplaysCompletedRequest(t *testing.T) {
	appointmentID := testUUID(9)
	store := &appointmentStoreStub{
		claimRows: 0,
		claimSet:  true,
		idempotency: db.IdempotencyRequest{
			RequestHash:   "placeholder",
			Status:        "completed",
			AppointmentID: appointmentID,
		},
	}
	appointmentService := NewAppointmentService(store)
	input := CreateAppointmentInput{CustomerID: testUUID(3), VehicleID: testUUID(4), DealershipID: testUUID(5), ServiceTypeID: testUUID(6), TechnicianID: testUUID(1), ServiceBayID: testUUID(2), StartTimeUtc: testTime(9), EndTimeUtc: testTime(10)}
	hash, err := hashCreateAppointmentInput(input)
	if err != nil {
		t.Fatalf("hashCreateAppointmentInput returned error: %v", err)
	}
	store.idempotency.RequestHash = hash
	store.appointment = db.Appointment{ID: appointmentID, Status: "confirmed"}

	appointment, err := appointmentService.CreateAppointment(context.Background(), "booking-1", input)
	if err != nil {
		t.Fatalf("CreateAppointment returned error: %v", err)
	}
	if appointment.ID != appointmentID {
		t.Fatalf("expected replayed appointment %v, got %v", appointmentID, appointment.ID)
	}
	if store.createCalled {
		t.Fatal("expected completed request to skip appointment creation")
	}
}

func TestCreateAppointmentRejectsIdempotencyConflict(t *testing.T) {
	store := &appointmentStoreStub{
		claimRows:   0,
		claimSet:    true,
		idempotency: db.IdempotencyRequest{RequestHash: "different", Status: "completed"},
	}
	appointmentService := NewAppointmentService(store)

	_, err := appointmentService.CreateAppointment(context.Background(), "booking-1", CreateAppointmentInput{
		CustomerID: testUUID(3), VehicleID: testUUID(4), DealershipID: testUUID(5), ServiceTypeID: testUUID(6),
		TechnicianID: testUUID(1), ServiceBayID: testUUID(2), StartTimeUtc: testTime(9), EndTimeUtc: testTime(10),
	})
	if err != ErrIdempotencyConflict {
		t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
	}
}

func testUUID(first byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{first}, Valid: true}
}

func testTime(hour int) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Date(2026, 8, 12, hour, 0, 0, 0, time.UTC), Valid: true}
}
