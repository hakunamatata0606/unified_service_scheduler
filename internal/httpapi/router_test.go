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

	"github.com/hakunamatata0606/unified_service_scheduler/internal/database"
	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
	"github.com/hakunamatata0606/unified_service_scheduler/internal/service"
)

type appointmentStoreStub struct {
	appointment          db.Appointment
	appointments         []db.Appointment
	customerAppointments []service.AppointmentDetail
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
	gotCustomerID        pgtype.UUID
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

func (s *appointmentStoreStub) GetAvailability(ctx context.Context, dealershipID, serviceTypeID pgtype.UUID, start pgtype.Timestamptz) (service.Availability, error) {
	s.gotContext = ctx
	s.gotAvailabilityInput = true
	return service.Availability{
		Available:       len(s.technicians) > 0 && len(s.serviceBays) > 0,
		DurationMinutes: 90,
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

func (s *appointmentStoreStub) ListAppointmentDetails(context.Context, pgtype.UUID, pgtype.Timestamptz, pgtype.Timestamptz) ([]service.AppointmentDetail, error) {
	return nil, s.err
}

func (s *appointmentStoreStub) ListCustomerAppointments(_ context.Context, customerID pgtype.UUID) ([]service.AppointmentDetail, error) {
	s.gotCustomerID = customerID
	return s.customerAppointments, s.err
}

func (s *appointmentStoreStub) ListTechnicianAvailability(context.Context, pgtype.UUID, pgtype.UUID, pgtype.Timestamptz, pgtype.Timestamptz) ([]service.TechnicianAvailability, error) {
	return nil, s.err
}

func (s *appointmentStoreStub) ListVehicles(context.Context, pgtype.UUID) ([]db.Vehicle, error) {
	return nil, s.err
}

func (s *appointmentStoreStub) ListServiceTypes(context.Context) ([]db.ServiceType, error) {
	return nil, s.err
}

func (s *appointmentStoreStub) ListDealerships(context.Context) ([]db.Dealership, error) {
	return nil, s.err
}

type authServiceStub struct {
	user database.User
}

func (s *authServiceStub) Login(context.Context, string, string) (database.User, error) {
	return s.user, nil
}

func (s *authServiceStub) GetUser(context.Context, pgtype.UUID) (database.User, error) {
	return s.user, nil
}

func (s *authServiceStub) ListVehicles(context.Context, pgtype.UUID) ([]db.Vehicle, error) {
	return nil, nil
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

func TestWebConsole(t *testing.T) {
	router := NewRouter(nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Unified Service Scheduler") {
		t.Fatal("expected embedded web console")
	}
}

func TestAdminPage(t *testing.T) {
	router := NewRouter(nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Appointments") {
		t.Fatal("expected admin page")
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
		"requestedStart":"2026-08-12T02:00:00Z"
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
	if store.gotCreateInput.CustomerID != ids.customer || store.gotCreateInput.VehicleID != testUUID(2) || store.gotCreateInput.DealershipID != ids.dealership {
		t.Fatalf("unexpected create input: %+v", store.gotCreateInput)
	}
	if !store.gotCreateInput.StartTimeUtc.Valid || store.gotCreateInput.EndTimeUtc.Valid {
		t.Fatal("expected only the requested start time from the HTTP handler")
	}
	if store.gotCreateInput.TechnicianID.Valid || store.gotCreateInput.ServiceBayID.Valid {
		t.Fatal("expected technician and service bay to be assigned by the backend")
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
	request := httptest.NewRequest(http.MethodGet, "/api/v1/availability?dealershipId=03000000-0000-0000-0000-000000000000&serviceTypeId=04000000-0000-0000-0000-000000000000&start=2026-08-12T02:00:00Z", nil)

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

func TestListCustomerAppointments(t *testing.T) {
	customerID := testUUID(1)
	userID := testUUID(8)
	appointments := []service.AppointmentDetail{{
		Appointment: db.Appointment{ID: testUUID(7), CustomerID: customerID, Status: "confirmed"},
		VehicleMake: "Toyota", VehicleModel: "Camry", ServiceTypeName: "Oil change",
	}}
	store := &appointmentStoreStub{customerAppointments: appointments}
	auth := &authServiceStub{user: database.User{ID: userID, CustomerID: customerID, Email: "demo@example.com"}}
	router := NewRouter(store, auth)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/my/appointments", nil)
	request.AddCookie(&http.Cookie{Name: "session_user", Value: userID.String()})

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != mustJSON(appointments) {
		t.Fatalf("expected body %s, got %s", mustJSON(appointments), recorder.Body.String())
	}
	if store.gotCustomerID != customerID {
		t.Fatalf("expected customer %v, got %v", customerID, store.gotCustomerID)
	}
}

func TestListCustomerAppointmentsRequiresLogin(t *testing.T) {
	router := NewRouter(&appointmentStoreStub{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/my/appointments", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
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
