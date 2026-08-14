package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
)

func (s *Store) ListVehicles(ctx context.Context, customerID pgtype.UUID) ([]db.Vehicle, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, customer_id, vin, make, model, created_at_utc FROM vehicles WHERE customer_id = $1 ORDER BY make, model, id`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]db.Vehicle, 0)
	for rows.Next() {
		var item db.Vehicle
		if err := rows.Scan(&item.ID, &item.CustomerID, &item.Vin, &item.Make, &item.Model, &item.CreatedAtUtc); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListServiceTypes(ctx context.Context) ([]db.ServiceType, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, duration_minutes, required_skill, created_at_utc FROM service_types ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]db.ServiceType, 0)
	for rows.Next() {
		var item db.ServiceType
		if err := rows.Scan(&item.ID, &item.Name, &item.DurationMinutes, &item.RequiredSkill, &item.CreatedAtUtc); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetServiceType(ctx context.Context, id pgtype.UUID) (db.ServiceType, error) {
	var item db.ServiceType
	err := s.pool.QueryRow(ctx, `SELECT id, name, duration_minutes, required_skill, created_at_utc FROM service_types WHERE id = $1`, id).Scan(
		&item.ID, &item.Name, &item.DurationMinutes, &item.RequiredSkill, &item.CreatedAtUtc,
	)
	return item, err
}

func (s *Store) ListDealerships(ctx context.Context) ([]db.Dealership, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, timezone, created_at_utc FROM dealerships ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]db.Dealership, 0)
	for rows.Next() {
		var item db.Dealership
		if err := rows.Scan(&item.ID, &item.Name, &item.Timezone, &item.CreatedAtUtc); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type AppointmentDetail struct {
	db.Appointment
	CustomerName    string `json:"customer_name"`
	UserEmail       string `json:"user_email"`
	VehicleMake     string `json:"vehicle_make"`
	VehicleModel    string `json:"vehicle_model"`
	DealershipName  string `json:"dealership_name"`
	TechnicianName  string `json:"technician_name"`
	ServiceBayName  string `json:"service_bay_name"`
	ServiceTypeName string `json:"service_type_name"`
}

type TechnicianAvailability struct {
	ID        pgtype.UUID `json:"id"`
	Name      string      `json:"name"`
	Skills    []string    `json:"skills"`
	Available bool        `json:"available"`
}

func (s *Store) ListTechnicianAvailability(ctx context.Context, dealershipID pgtype.UUID, serviceTypeID pgtype.UUID, start, end pgtype.Timestamptz) ([]TechnicianAvailability, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.name,
		       COALESCE(array_agg(ts.skill ORDER BY ts.skill) FILTER (WHERE ts.skill IS NOT NULL), ARRAY[]::text[]),
		       EXISTS (
			       SELECT 1
			       FROM technician_skills qualified
			       JOIN service_types st ON st.required_skill = qualified.skill
			       WHERE qualified.technician_id = t.id
			         AND st.id = $2
		       ) AND NOT EXISTS (
			       SELECT 1
			       FROM appointments a
			       WHERE a.technician_id = t.id
			         AND a.status = 'confirmed'
			         AND a.start_time_utc < $3
			         AND a.end_time_utc > $4
		       ) AS available
		FROM technicians t
		LEFT JOIN technician_skills ts ON ts.technician_id = t.id
		WHERE t.dealership_id = $1
		GROUP BY t.id, t.name
		ORDER BY t.name`, dealershipID, serviceTypeID, end, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]TechnicianAvailability, 0)
	for rows.Next() {
		var item TechnicianAvailability
		if err := rows.Scan(&item.ID, &item.Name, &item.Skills, &item.Available); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) ListAppointmentDetails(ctx context.Context, dealershipID pgtype.UUID, start, end pgtype.Timestamptz) ([]AppointmentDetail, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.customer_id, a.vehicle_id, a.dealership_id,
		       a.technician_id, a.service_bay_id, a.service_type_id,
		       a.start_time_utc, a.end_time_utc, a.status, a.created_at_utc,
		       c.name, COALESCE(u.email, c.email), v.make, v.model, d.name, t.name, sb.name, st.name
		FROM appointments a
		JOIN customers c ON c.id = a.customer_id
		LEFT JOIN users u ON u.customer_id = c.id
		JOIN vehicles v ON v.id = a.vehicle_id
		JOIN dealerships d ON d.id = a.dealership_id
		JOIN technicians t ON t.id = a.technician_id
		JOIN service_bays sb ON sb.id = a.service_bay_id
		JOIN service_types st ON st.id = a.service_type_id
		WHERE a.dealership_id = $1
		  AND a.start_time_utc < $2
		  AND a.end_time_utc > $3
		ORDER BY a.start_time_utc, a.id`, dealershipID, end, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AppointmentDetail, 0)
	for rows.Next() {
		var item AppointmentDetail
		if err := rows.Scan(
			&item.ID, &item.CustomerID, &item.VehicleID, &item.DealershipID,
			&item.TechnicianID, &item.ServiceBayID, &item.ServiceTypeID,
			&item.StartTimeUtc, &item.EndTimeUtc, &item.Status, &item.CreatedAtUtc,
			&item.CustomerName, &item.UserEmail, &item.VehicleMake, &item.VehicleModel,
			&item.DealershipName, &item.TechnicianName, &item.ServiceBayName,
			&item.ServiceTypeName,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ListAppointmentDetailsForCustomer returns every appointment registered to a
// customer, newest first, with the resource names needed by the customer UI.
func (s *Store) ListAppointmentDetailsForCustomer(ctx context.Context, customerID pgtype.UUID) ([]AppointmentDetail, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.customer_id, a.vehicle_id, a.dealership_id,
		       a.technician_id, a.service_bay_id, a.service_type_id,
		       a.start_time_utc, a.end_time_utc, a.status, a.created_at_utc,
		       c.name, COALESCE(u.email, c.email), v.make, v.model, d.name, t.name, sb.name, st.name
		FROM appointments a
		JOIN customers c ON c.id = a.customer_id
		LEFT JOIN users u ON u.customer_id = c.id
		JOIN vehicles v ON v.id = a.vehicle_id
		JOIN dealerships d ON d.id = a.dealership_id
		JOIN technicians t ON t.id = a.technician_id
		JOIN service_bays sb ON sb.id = a.service_bay_id
		JOIN service_types st ON st.id = a.service_type_id
		WHERE a.customer_id = $1
		ORDER BY a.start_time_utc DESC, a.id`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AppointmentDetail, 0)
	for rows.Next() {
		var item AppointmentDetail
		if err := rows.Scan(
			&item.ID, &item.CustomerID, &item.VehicleID, &item.DealershipID,
			&item.TechnicianID, &item.ServiceBayID, &item.ServiceTypeID,
			&item.StartTimeUtc, &item.EndTimeUtc, &item.Status, &item.CreatedAtUtc,
			&item.CustomerName, &item.UserEmail, &item.VehicleMake, &item.VehicleModel,
			&item.DealershipName, &item.TechnicianName, &item.ServiceBayName,
			&item.ServiceTypeName,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
