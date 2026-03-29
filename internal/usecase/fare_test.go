package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"github.com/AbinterDon/robotaxi-system-design/internal/usecase"
)

// ─── Mock (golang-testing: interface-based mocking) ──────────────────────────

type mockFareRepo struct {
	saved  *domain.Fare
	findFn func(id string) (*domain.Fare, error)
}

func (m *mockFareRepo) Save(_ context.Context, f *domain.Fare) error {
	m.saved = f
	return nil
}

func (m *mockFareRepo) FindByID(_ context.Context, id string) (*domain.Fare, error) {
	if m.findFn != nil {
		return m.findFn(id)
	}
	return nil, domain.ErrFareNotFound
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestFareUseCase_EstimateFare(t *testing.T) {
	t.Parallel()

	// Table-driven tests (golang-testing skill)
	tests := []struct {
		name    string
		pickup  domain.Location
		dest    domain.Location
		wantMin float64 // minimum expected fare
		wantMax float64 // maximum expected fare
		wantErr bool
	}{
		{
			name:    "same location",
			pickup:  domain.Location{Lat: 37.7749, Lng: -122.4194},
			dest:    domain.Location{Lat: 37.7749, Lng: -122.4194},
			wantMin: 2.0, // base fare only
			wantMax: 2.5,
		},
		{
			name:    "short trip ~1km",
			pickup:  domain.Location{Lat: 37.7749, Lng: -122.4194},
			dest:    domain.Location{Lat: 37.7839, Lng: -122.4094},
			wantMin: 3.0,
			wantMax: 6.0,
		},
		{
			name:    "longer trip ~10km",
			pickup:  domain.Location{Lat: 37.7749, Lng: -122.4194},
			dest:    domain.Location{Lat: 37.8449, Lng: -122.2894},
			wantMin: 15.0,
			wantMax: 30.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &mockFareRepo{}
			uc := usecase.NewFareUseCase(repo)

			fare, err := uc.EstimateFare(context.Background(), tt.pickup, tt.dest)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fare.ID == "" {
				t.Error("expected fare ID to be set")
			}
			if fare.EstimatedFare < tt.wantMin || fare.EstimatedFare > tt.wantMax {
				t.Errorf("fare %.2f not in range [%.2f, %.2f]", fare.EstimatedFare, tt.wantMin, tt.wantMax)
			}
			if fare.DistanceKm < 0 {
				t.Errorf("distance must be non-negative, got %.2f", fare.DistanceKm)
			}
			if fare.EstimatedDurationMinutes < 5 {
				t.Errorf("duration must be >= 5 (pickup buffer), got %d", fare.EstimatedDurationMinutes)
			}
			if repo.saved == nil {
				t.Error("expected fare to be persisted")
			}
		})
	}
}

func TestFareUseCase_EstimateFare_RepoError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db unavailable")
	uc := usecase.NewFareUseCase(&failSaveFareRepo{err: wantErr})

	_, err := uc.EstimateFare(context.Background(),
		domain.Location{Lat: 37.7749, Lng: -122.4194},
		domain.Location{Lat: 37.7839, Lng: -122.4094},
	)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

type failSaveFareRepo struct{ err error }

func (r *failSaveFareRepo) Save(_ context.Context, _ *domain.Fare) error      { return r.err }
func (r *failSaveFareRepo) FindByID(_ context.Context, _ string) (*domain.Fare, error) {
	return nil, r.err
}
