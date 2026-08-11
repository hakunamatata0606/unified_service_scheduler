package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
	"github.com/hakunamatata0606/unified_service_scheduler/internal/service"
)

type appointmentStoreStub struct {
	appointment          db.Appointment
	appointments         []db.Appointment
	technicians          []db.Technician
	serviceBays          []db.ServiceBay
	err                  error
	called               bool
	gotContext           context.Context
	gotID                pgtype.UUID
	gotCreateInput       service.CreateAppointmentInput
	gotListDealershipID  pgtype.UUID
	gotListStart         pgtype.Timestamptz
	gotListEnd           pgtype.Timestamptz
	gotAvailabilityInput bool
	idempotency          db.IdempotencyRequest
}

func (s *appointmentStoreStub) GetAppointment(ctx context.Context, id pgtype.UUID) (db.Appointment, error) {
	s.called = true
	s.gotContext = ctx
	s.gotID = id
	return s.appointment, s.err
}

func (s *appointmentStoreStub) CreateAppointment(ctx context.Context, key string, input service.CreateAppointmentInput) (db.Appointment, error) {
	s.gotContext = ctx
	s.gotCreateInput = input
	return s.appointment, s.err
}

func (s *appointmentStoreStub) CreateIdempotencyRequest(context.Context, db.CreateIdempotencyRequestParams) (int64, error) {
	return 1, s.err
}

func (s *appointmentStoreStub) GetIdempotencyRequest(context.Context, string) (db.IdempotencyRequest, error) {
	return s.idempotency, s.err
}

func (s *appointmentStoreStub) CompleteIdempotencyRequest(context.Context, db.CompleteIdempotencyRequestParams) (db.IdempotencyRequest, error) {
	return s.idempotency, s.err
}

func (s *appointmentStoreStub) GetAvailability(ctx context.Context, dealershipID, serviceTypeID pgtype.UUID, start, end pgtype.Timestamptz) (service.Availability, error) {
	s.gotContext = ctx
	s.gotAvailabilityInput = true
	return service.Availability{
		Available:       len(s.technicians) > 0 && len(s.serviceBays) > 0,
		DurationMinutes: int(end.Time.Sub(start.Time).Minutes()),
		Technicians:     s.technicians,
		ServiceBays:     s.serviceBays,
	}, s.err
}

func (s *appointmentStoreStub) ListAppointments(ctx context.Context, dealershipID pgtype.UUID, start, end pgtype.Timestamptz) ([]db.Appointment, error) {
	s.gotContext = ctx
	s.gotListDealershipID = dealershipID
	s.gotListStart = start
	s.gotListEnd = end
	return s.appointments, s.err
}

func TestHealth(t *testing.T) {
	router := NewRouter(nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if recorder.Body.String() != "{\"status\":\"ok\"}" {
		t.Fatalf("unexpected response body: %s", recorder.Body.String())
	}
}

func TestGetAppointment(t *testing.T) {
	appointmentID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	appointment := db.Appointment{ID: appointmentID, Status: "confirmed"}

	tests := []struct {
		name           string
		path           string
		appointment    db.Appointment
		err            error
		wantStatus     int
		wantBody       string
		wantMockCalled bool
	}{
		{
			name:           "found",
			path:           "/api/v1/appointments/01020300-0000-0000-0000-000000000000",
			appointment:    appointment,
			wantStatus:     http.StatusOK,
			wantBody:       mustJSON(appointment),
			wantMockCalled: true,
		},
		{
			name:       "malformed uuid",
			path:       "/api/v1/appointments/not-a-uuid",
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid appointment id"}`,
		},
		{
			name:           "not found",
			path:           "/api/v1/appointments/01020300-0000-0000-0000-000000000000",
			err:            pgx.ErrNoRows,
			wantStatus:     http.StatusNotFound,
			wantBody:       `{"error":"appointment not found"}`,
			wantMockCalled: true,
		},
		{
			name:           "database error",
			path:           "/api/v1/appointments/01020300-0000-0000-0000-000000000000",
			err:            errors.New("database unavailable"),
			wantStatus:     http.StatusInternalServerError,
			wantBody:       `{"error":"internal server error"}`,
			wantMockCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &appointmentStoreStub{appointment: tt.appointment, err: tt.err}
			router := NewRouter(store)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, recorder.Code)
			}
			if recorder.Body.String() != tt.wantBody {
				t.Fatalf("expected body %s, got %s", tt.wantBody, recorder.Body.String())
			}
			if store.called != tt.wantMockCalled {
				t.Fatalf("expected mock called=%t, got %t", tt.wantMockCalled, store.called)
			}
			if tt.wantMockCalled {
				if store.gotContext != request.Context() {
					t.Fatal("expected handler to pass request context to store")
				}
				if store.gotID != appointmentID {
					t.Fatalf("expected id %v, got %v", appointmentID, store.gotID)
				}
			}
		})
	}
}

func TestCreateAppointment(t *testing.T) {
	ids := testIDs()
	created := db.Appointment{ID: ids.appointment, Status: "confirmed"}
	store := &appointmentStoreStub{appointment: created}
	router := NewRouter(store)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", strings.NewReader(`{
		"customerId":"01000000-0000-0000-0000-000000000000",
		"vehicleId":"02000000-0000-0000-0000-000000000000",
		"dealershipId":"03000000-0000-0000-0000-000000000000",
		"serviceTypeId":"04000000-0000-0000-0000-000000000000",
		"technicianId":"05000000-0000-0000-0000-000000000000",
		"serviceBayId":"06000000-0000-0000-0000-000000000000",
		"requestedStart":"2026-08-12T02:00:00Z",
		"requestedEnd":"2026-08-12T03:30:00Z"
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "create-appointment-test")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if recorder.Body.String() != mustJSON(created) {
		t.Fatalf("expected body %s, got %s", mustJSON(created), recorder.Body.String())
	}
	if store.gotCreateInput.CustomerID != ids.customer || store.gotCreateInput.TechnicianID != ids.technician || store.gotCreateInput.ServiceBayID != ids.serviceBay {
		t.Fatalf("unexpected create input: %+v", store.gotCreateInput)
	}
	if !store.gotCreateInput.StartTimeUtc.Valid || !store.gotCreateInput.EndTimeUtc.Valid {
		t.Fatal("expected valid appointment interval")
	}
}

func TestListAppointments(t *testing.T) {
	ids := testIDs()
	appointments := []db.Appointment{{ID: ids.appointment, Status: "confirmed"}}
	store := &appointmentStoreStub{appointments: appointments}
	router := NewRouter(store)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/appointments?dealershipId=03000000-0000-0000-0000-000000000000&from=2026-08-12T00:00:00Z&to=2026-08-13T00:00:00Z", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if recorder.Body.String() != mustJSON(appointments) {
		t.Fatalf("expected body %s, got %s", mustJSON(appointments), recorder.Body.String())
	}
	if store.gotListDealershipID != ids.dealership {
		t.Fatalf("unexpected dealership id: %v", store.gotListDealershipID)
	}
}

func TestGetAvailability(t *testing.T) {
	ids := testIDs()
	technicians := []db.Technician{{ID: ids.technician, Name: "Alex"}}
	bays := []db.ServiceBay{{ID: ids.serviceBay, Name: "Bay 1"}}
	store := &appointmentStoreStub{technicians: technicians, serviceBays: bays}
	router := NewRouter(store)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/availability?dealershipId=03000000-0000-0000-0000-000000000000&serviceTypeId=04000000-0000-0000-0000-000000000000&start=2026-08-12T02:00:00Z&end=2026-08-12T03:30:00Z", nil)

	router.ServeHTTP(recorder, request)

	want := service.Availability{Available: true, DurationMinutes: 90, Technicians: technicians, ServiceBays: bays}
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if recorder.Body.String() != mustJSON(want) {
		t.Fatalf("expected body %s, got %s", mustJSON(want), recorder.Body.String())
	}
	if !store.gotAvailabilityInput {
		t.Fatal("expected availability queries to be called")
	}
}

type testAppointmentIDs struct {
	appointment, customer, dealership, technician, serviceBay pgtype.UUID
}

func testIDs() testAppointmentIDs {
	return testAppointmentIDs{
		appointment: testUUID(7),
		customer:    testUUID(1),
		dealership:  testUUID(3),
		technician:  testUUID(5),
		serviceBay:  testUUID(6),
	}
}

func testUUID(first byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{first}, Valid: true}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}
