package usecase

import (
	"context"
	"fmt"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"github.com/google/uuid"
)

type RideUseCase struct {
	rideRepo domain.RideRepository
	fareRepo domain.FareRepository
	queue    domain.RideQueue
}

func NewRideUseCase(rideRepo domain.RideRepository, fareRepo domain.FareRepository, queue domain.RideQueue) *RideUseCase {
	return &RideUseCase{rideRepo: rideRepo, fareRepo: fareRepo, queue: queue}
}

// CreateRide creates a ride from a confirmed fare and publishes it to the matching queue.
// Deep Dive 2: Ride Service publishes and returns immediately; matching happens async.
func (uc *RideUseCase) CreateRide(ctx context.Context, fareID string) (*domain.Ride, error) {
	fare, err := uc.fareRepo.FindByID(ctx, fareID)
	if err != nil {
		return nil, fmt.Errorf("create ride: %w", err)
	}

	ride := &domain.Ride{
		ID:             uuid.NewString(),
		FareID:         fare.ID,
		PickupLocation: fare.PickupLocation,
		Destination:    fare.Destination,
		EstimatedFare:  fare.EstimatedFare,
		Status:         domain.StatusPending,
	}

	if err := uc.rideRepo.Save(ctx, ride); err != nil {
		return nil, fmt.Errorf("create ride: %w", err)
	}

	if err := uc.queue.Publish(ctx, ride.ID); err != nil {
		return nil, fmt.Errorf("create ride: publish to queue: %w", err)
	}

	return ride, nil
}

func (uc *RideUseCase) GetRide(ctx context.Context, rideID string) (*domain.Ride, error) {
	ride, err := uc.rideRepo.FindByID(ctx, rideID)
	if err != nil {
		return nil, fmt.Errorf("get ride: %w", err)
	}
	return ride, nil
}
