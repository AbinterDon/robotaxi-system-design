package domain

import "context"

// ─── Repository Ports (inbound to domain) ────────────────────────────────────

// FareRepository defines data access for fare estimates.
type FareRepository interface {
	Save(ctx context.Context, fare *Fare) error
	FindByID(ctx context.Context, id string) (*Fare, error)
}

// RideRepository defines data access for rides.
type RideRepository interface {
	Save(ctx context.Context, ride *Ride) error
	FindByID(ctx context.Context, id string) (*Ride, error)
	UpdateStatus(ctx context.Context, id string, status RideStatus) error
	// AssignAV updates the ride with an AV assignment.
	// Returns ErrAVAlreadyBusy if av_id already has an active ride (Deep Dive 4).
	AssignAV(ctx context.Context, rideID, avID, licensePlate string) error
}

// ─── Service Ports (outbound from domain) ────────────────────────────────────

// LocationStore is the port for AV location management (backed by Redis).
type LocationStore interface {
	UpdateLocation(ctx context.Context, av AVLocation) error
	FindNearbyAvailable(ctx context.Context, pickup Location, radiusKm float64, limit int) ([]string, error)
	MarkBusy(ctx context.Context, avID string) error
}

// DispatchGateway is the port for sending/receiving AV dispatch commands.
type DispatchGateway interface {
	SendCommand(ctx context.Context, avID string, cmd DispatchCommand) error
	PollCommand(ctx context.Context, avID string) (*DispatchCommand, error)
	SubmitDecision(ctx context.Context, rideID, avID string, decision DispatchDecision) error
	WaitDecision(ctx context.Context, rideID, avID string) (DispatchDecision, error)
}

// MatchingStateStore handles distributed matching state and locks (Deep Dive 3).
type MatchingStateStore interface {
	CreateState(ctx context.Context, rideID string, candidates []string) error
	AcquireLock(ctx context.Context, rideID string) (bool, error)
	ReleaseLock(ctx context.Context, rideID string) error
	GetStatus(ctx context.Context, rideID string) (string, error)
	MarkDone(ctx context.Context, rideID string) error
}

// RideQueue is the port for the async message queue between Ride and Matching services (Deep Dive 2).
type RideQueue interface {
	Publish(ctx context.Context, rideID string) error
	Consume(ctx context.Context) (string, error)
}
