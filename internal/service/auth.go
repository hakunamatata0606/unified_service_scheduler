package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/hakunamatata0606/unified_service_scheduler/internal/database"
	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

type AuthService struct {
	store interface {
		GetUserByEmail(context.Context, string) (database.User, error)
		GetUser(context.Context, pgtype.UUID) (database.User, error)
		ListVehiclesForUser(context.Context, pgtype.UUID) ([]db.Vehicle, error)
	}
}

func NewAuthService(store AuthServiceStore) *AuthService {
	return &AuthService{store: store}
}

type AuthServiceStore interface {
	GetUserByEmail(context.Context, string) (database.User, error)
	GetUser(context.Context, pgtype.UUID) (database.User, error)
	ListVehiclesForUser(context.Context, pgtype.UUID) ([]db.Vehicle, error)
}

func (s *AuthService) Login(ctx context.Context, email, password string) (database.User, error) {
	user, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.User{}, ErrInvalidCredentials
		}
		return database.User{}, err
	}
	hash := sha256.Sum256([]byte(user.PasswordSalt + ":" + password))
	if hex.EncodeToString(hash[:]) != user.PasswordHash {
		return database.User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *AuthService) GetUser(ctx context.Context, id pgtype.UUID) (database.User, error) {
	return s.store.GetUser(ctx, id)
}

func (s *AuthService) ListVehicles(ctx context.Context, userID pgtype.UUID) ([]db.Vehicle, error) {
	return s.store.ListVehiclesForUser(ctx, userID)
}
