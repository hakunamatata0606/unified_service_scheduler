package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
)

var ErrNoAvailability = errors.New("no appointment availability")
var ErrVehicleNotOwned = errors.New("vehicle does not belong to customer")
var ErrIdempotencyConflict = errors.New("idempotency key was reused with a different request")
var ErrIdempotencyInProgress = errors.New("idempotency request is already in progress")

type BookAppointmentParams struct {
	ID            pgtype.UUID
	CustomerID    pgtype.UUID
	VehicleID     pgtype.UUID
	DealershipID  pgtype.UUID
	ServiceTypeID pgtype.UUID
	StartTimeUtc  pgtype.Timestamptz
	Key           string
	RequestHash   string
}

// BookAppointment performs the availability check, resource assignment,
// appointment insert, and idempotency completion atomically.
func (s *Store) BookAppointment(ctx context.Context, params BookAppointmentParams) (appointment db.Appointment, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return db.Appointment{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	queries := s.Queries.WithTx(tx)
	claimed, err := queries.CreateIdempotencyRequest(ctx, db.CreateIdempotencyRequestParams{
		Key: params.Key, RequestHash: params.RequestHash,
		ExpiresAtUtc: pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true},
	})
	if err != nil {
		return db.Appointment{}, err
	}
	if claimed == 0 {
		existing, getErr := queries.GetIdempotencyRequest(ctx, params.Key)
		if getErr != nil {
			return db.Appointment{}, getErr
		}
		if existing.RequestHash != params.RequestHash {
			return db.Appointment{}, ErrIdempotencyConflict
		}
		if existing.Status != "completed" || !existing.AppointmentID.Valid {
			return db.Appointment{}, ErrIdempotencyInProgress
		}
		appointment, err = queries.GetAppointment(ctx, existing.AppointmentID)
		if err != nil {
			return db.Appointment{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return db.Appointment{}, err
		}
		return appointment, nil
	}

	var ownsVehicle bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vehicles WHERE id = $1 AND customer_id = $2)`, params.VehicleID, params.CustomerID).Scan(&ownsVehicle); err != nil {
		return db.Appointment{}, err
	}
	if !ownsVehicle {
		return db.Appointment{}, ErrVehicleNotOwned
	}

	var durationMinutes int32
	if err = tx.QueryRow(ctx, `SELECT duration_minutes FROM service_types WHERE id = $1`, params.ServiceTypeID).Scan(&durationMinutes); err != nil {
		return db.Appointment{}, err
	}
	end := pgtype.Timestamptz{Time: params.StartTimeUtc.Time.Add(time.Duration(durationMinutes) * time.Minute), Valid: true}

	technicians, err := queries.FindAvailableTechnicians(ctx, db.FindAvailableTechniciansParams{
		ServiceTypeID: params.ServiceTypeID, DealershipID: params.DealershipID,
		RequestedStart: params.StartTimeUtc, RequestedEnd: end,
	})
	if err != nil {
		return db.Appointment{}, err
	}
	bays, err := queries.FindAvailableServiceBays(ctx, db.FindAvailableServiceBaysParams{
		ServiceTypeID: params.ServiceTypeID, DealershipID: params.DealershipID,
		RequestedStart: params.StartTimeUtc, RequestedEnd: end,
	})
	if err != nil {
		return db.Appointment{}, err
	}
	if len(technicians) == 0 || len(bays) == 0 {
		return db.Appointment{}, ErrNoAvailability
	}

	appointment, err = queries.CreateAppointment(ctx, db.CreateAppointmentParams{
		ID: params.ID, CustomerID: params.CustomerID, VehicleID: params.VehicleID,
		DealershipID: params.DealershipID, TechnicianID: technicians[0].ID,
		ServiceBayID: bays[0].ID, ServiceTypeID: params.ServiceTypeID,
		StartTimeUtc: params.StartTimeUtc, EndTimeUtc: end,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return db.Appointment{}, ErrNoAvailability
		}
		return db.Appointment{}, err
	}
	responseBody, err := json.Marshal(appointment)
	if err != nil {
		return db.Appointment{}, err
	}
	if _, err = queries.CompleteIdempotencyRequest(ctx, db.CompleteIdempotencyRequestParams{
		AppointmentID: appointment.ID, ResponseStatus: pgtype.Int4{Int32: 201, Valid: true},
		ResponseBody: responseBody, Key: params.Key,
	}); err != nil {
		return db.Appointment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return db.Appointment{}, err
	}
	return appointment, nil
}
