package usecase_test

import (
	"context"
	"testing"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"github.com/AbinterDon/robotaxi-system-design/internal/usecase"
)

// ─── Mocks ────────────────────────────────────────────────────────────────────

type mockRideRepo struct {
	rides    map[string]*domain.Ride
	statuses map[string]domain.RideStatus
}

func newMockRideRepo() *mockRideRepo {
	return &mockRideRepo{rides: make(map[string]*domain.Ride), statuses: make(map[string]domain.RideStatus)}
}

func (m *mockRideRepo) Save(_ context.Context, r *domain.Ride) error {
	m.rides[r.ID] = r
	return nil
}
func (m *mockRideRepo) FindByID(_ context.Context, id string) (*domain.Ride, error) {
	if r, ok := m.rides[id]; ok {
		return r, nil
	}
	return nil, domain.ErrRideNotFound
}
func (m *mockRideRepo) UpdateStatus(_ context.Context, id string, s domain.RideStatus) error {
	m.statuses[id] = s
	return nil
}
func (m *mockRideRepo) AssignAV(_ context.Context, rideID, avID, plate string) error {
	if r, ok := m.rides[rideID]; ok {
		r.AVID = avID
		r.AVLicensePlate = plate
		r.Status = domain.StatusDriverAssigned
	}
	return nil
}

type mockQueue struct{ published []string }

func (q *mockQueue) Publish(_ context.Context, rideID string) error {
	q.published = append(q.published, rideID)
	return nil
}
func (q *mockQueue) Consume(_ context.Context) (string, error) { return "", nil }

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestRideUseCase_CreateRide(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fareID  string
		setupFn func(fareRepo *mockFareRepo)
		wantErr bool
	}{
		{
			name:   "valid fare",
			fareID: "fare-123",
			setupFn: func(r *mockFareRepo) {
				r.findFn = func(id string) (*domain.Fare, error) {
					if id == "fare-123" {
						return &domain.Fare{
							ID:            "fare-123",
							EstimatedFare: 10.0,
							PickupLocation: domain.Location{Lat: 37.77, Lng: -122.42},
							Destination:   domain.Location{Lat: 37.78, Lng: -122.41},
						}, nil
					}
					return nil, domain.ErrFareNotFound
				}
			},
		},
		{
			name:    "fare not found",
			fareID:  "nonexistent",
			setupFn: func(r *mockFareRepo) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fareRepo := &mockFareRepo{}
			tt.setupFn(fareRepo)
			rideRepo := newMockRideRepo()
			q := &mockQueue{}

			uc := usecase.NewRideUseCase(rideRepo, fareRepo, q)
			ride, err := uc.CreateRide(context.Background(), tt.fareID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ride.ID == "" {
				t.Error("expected ride ID to be set")
			}
			if ride.Status != domain.StatusPending {
				t.Errorf("got status %q; want %q", ride.Status, domain.StatusPending)
			}
			if len(q.published) != 1 || q.published[0] != ride.ID {
				t.Errorf("ride %q not published to queue", ride.ID)
			}
		})
	}
}

func TestRideUseCase_GetRide(t *testing.T) {
	t.Parallel()

	rideRepo := newMockRideRepo()
	rideRepo.rides["r1"] = &domain.Ride{ID: "r1", Status: domain.StatusMatching}

	uc := usecase.NewRideUseCase(rideRepo, &mockFareRepo{}, &mockQueue{})

	t.Run("found", func(t *testing.T) {
		ride, err := uc.GetRide(context.Background(), "r1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ride.ID != "r1" {
			t.Errorf("got id %q; want %q", ride.ID, "r1")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := uc.GetRide(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("expected ErrRideNotFound, got nil")
		}
	})
}
