package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
)

type User struct {
	ID           pgtype.UUID `json:"id"`
	CustomerID   pgtype.UUID `json:"customer_id"`
	Email        string      `json:"email"`
	PasswordSalt string
	PasswordHash string
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `SELECT id, customer_id, email, password_salt, password_hash FROM users WHERE lower(email) = lower($1)`, email).
		Scan(&user.ID, &user.CustomerID, &user.Email, &user.PasswordSalt, &user.PasswordHash)
	return user, err
}

func (s *Store) GetUser(ctx context.Context, id pgtype.UUID) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx, `SELECT id, customer_id, email, password_salt, password_hash FROM users WHERE id = $1`, id).
		Scan(&user.ID, &user.CustomerID, &user.Email, &user.PasswordSalt, &user.PasswordHash)
	return user, err
}

func (s *Store) ListVehiclesForUser(ctx context.Context, userID pgtype.UUID) ([]db.Vehicle, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT v.id, v.customer_id, v.vin, v.make, v.model, v.created_at_utc
		FROM vehicles v JOIN users u ON u.customer_id = v.customer_id
		WHERE u.id = $1 ORDER BY v.make, v.model, v.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	vehicles := make([]db.Vehicle, 0)
	for rows.Next() {
		var vehicle db.Vehicle
		if err := rows.Scan(&vehicle.ID, &vehicle.CustomerID, &vehicle.Vin, &vehicle.Make, &vehicle.Model, &vehicle.CreatedAtUtc); err != nil {
			return nil, err
		}
		vehicles = append(vehicles, vehicle)
	}
	return vehicles, rows.Err()
}
