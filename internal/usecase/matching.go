// Package usecase implements application business logic (use cases layer).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
)

const (
	maxSearchRadiusKm = 10.0
	maxCandidates     = 5
)

type MatchingUseCase struct {
	rideRepo     domain.RideRepository
	locationStore domain.LocationStore
	dispatch     domain.DispatchGateway
	matchState   domain.MatchingStateStore
	queue        domain.RideQueue
}

func NewMatchingUseCase(
	rideRepo domain.RideRepository,
	locationStore domain.LocationStore,
	dispatch domain.DispatchGateway,
	matchState domain.MatchingStateStore,
	queue domain.RideQueue,
) *MatchingUseCase {
	return &MatchingUseCase{
		rideRepo:     rideRepo,
		locationStore: locationStore,
		dispatch:     dispatch,
		matchState:   matchState,
		queue:        queue,
	}
}

// Run is a blocking worker that consumes ride requests from the queue and matches them.
// Deep Dive 2: Decoupled from Ride Service via message queue.
func (uc *MatchingUseCase) Run(ctx context.Context) {
	slog.Info("matching worker started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("matching worker stopped")
			return
		default:
		}

		rideID, err := uc.queue.Consume(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			continue // timeout — loop again
		}

		if err := uc.matchRide(ctx, rideID); err != nil {
			slog.Error("matching failed", "ride_id", rideID, "err", err)
		}
	}
}

func (uc *MatchingUseCase) matchRide(ctx context.Context, rideID string) error {
	ride, err := uc.rideRepo.FindByID(ctx, rideID)
	if err != nil {
		return fmt.Errorf("find ride: %w", err)
	}
	if ride.Status != domain.StatusPending {
		return nil // already handled
	}

	if err := uc.rideRepo.UpdateStatus(ctx, rideID, domain.StatusMatching); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	// Deep Dive 1: query Redis GEO for nearby available AVs
	candidates, err := uc.locationStore.FindNearbyAvailable(ctx, ride.PickupLocation, maxSearchRadiusKm, maxCandidates)
	if err != nil {
		return fmt.Errorf("find nearby avs: %w", err)
	}
	if len(candidates) == 0 {
		_ = uc.rideRepo.UpdateStatus(ctx, rideID, domain.StatusFailed)
		return domain.ErrNoAVAvailable
	}

	// Deep Dive 3: store candidate list + status in Redis for distributed workers
	if err := uc.matchState.CreateState(ctx, rideID, candidates); err != nil {
		return fmt.Errorf("create matching state: %w", err)
	}

	for _, avID := range candidates {
		matched, err := uc.tryDispatch(ctx, ride, avID)
		if err != nil {
			slog.Warn("dispatch attempt failed", "av_id", avID, "err", err)
			continue
		}
		if matched {
			return nil
		}
	}

	_ = uc.rideRepo.UpdateStatus(ctx, rideID, domain.StatusFailed)
	return fmt.Errorf("all candidates exhausted for ride %s", rideID)
}

// tryDispatch dispatches to one AV candidate and returns true if matched.
// Implements the distributed lock + state check described in Deep Dive 3.
func (uc *MatchingUseCase) tryDispatch(ctx context.Context, ride *domain.Ride, avID string) (bool, error) {
	// Acquire per-ride lock before reading/writing matching state
	ok, err := uc.matchState.AcquireLock(ctx, ride.ID)
	if err != nil {
		return false, fmt.Errorf("acquire lock: %w", err)
	}
	if !ok {
		return false, nil // another worker holds the lock
	}
	defer uc.matchState.ReleaseLock(ctx, ride.ID) //nolint:errcheck

	status, err := uc.matchState.GetStatus(ctx, ride.ID)
	if err != nil {
		return false, fmt.Errorf("get matching status: %w", err)
	}
	if status != "SEARCHING" {
		return false, nil // already matched by another worker
	}

	cmd := domain.DispatchCommand{
		RideID:         ride.ID,
		PickupLocation: ride.PickupLocation,
		Destination:    ride.Destination,
	}
	if err := uc.dispatch.SendCommand(ctx, avID, cmd); err != nil {
		return false, fmt.Errorf("send dispatch command: %w", err)
	}
	slog.Info("dispatched", "ride_id", ride.ID, "av_id", avID)

	// Release lock while waiting for AV response (don't block other workers)
	uc.matchState.ReleaseLock(ctx, ride.ID) //nolint:errcheck

	decision, err := uc.dispatch.WaitDecision(ctx, ride.ID, avID)
	if err != nil {
		return false, fmt.Errorf("wait decision: %w", err)
	}
	slog.Info("av decision", "av_id", avID, "ride_id", ride.ID, "decision", decision)

	if decision != domain.DecisionAccept {
		return false, nil
	}

	// Re-acquire lock before writing DONE (Deep Dive 3, step 4)
	ok, err = uc.matchState.AcquireLock(ctx, ride.ID)
	if err != nil || !ok {
		return false, fmt.Errorf("re-acquire lock after accept: %w", err)
	}
	defer uc.matchState.ReleaseLock(ctx, ride.ID) //nolint:errcheck

	// Verify still SEARCHING — prevents race when two AVs accept simultaneously
	status, _ = uc.matchState.GetStatus(ctx, ride.ID)
	if status != "SEARCHING" {
		return false, nil
	}

	// Deep Dive 4: DB uniqueness check — one AV can only have one active ride
	licensePlate := "TSLA-" + strings.ToUpper(avID[:4])
	if err := uc.rideRepo.AssignAV(ctx, ride.ID, avID, licensePlate); err != nil {
		if errors.Is(err, domain.ErrAVAlreadyBusy) {
			slog.Warn("av already busy", "av_id", avID)
			return false, nil
		}
		return false, fmt.Errorf("assign av: %w", err)
	}

	_ = uc.locationStore.MarkBusy(ctx, avID)
	_ = uc.matchState.MarkDone(ctx, ride.ID)
	slog.Info("ride matched", "ride_id", ride.ID, "av_id", avID)
	return true, nil
}
