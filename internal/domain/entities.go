// Package domain contains core business entities and rules.
// No dependencies on external frameworks or infrastructure.
package domain

import "errors"

// ─── Fare ────────────────────────────────────────────────────────────────────

type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Fare struct {
	ID                       string
	PickupLocation           Location
	Destination              Location
	EstimatedFare            float64
	EstimatedDurationMinutes int
	DistanceKm               float64
}

// ─── Ride ────────────────────────────────────────────────────────────────────

type RideStatus string

const (
	StatusPending        RideStatus = "PENDING"
	StatusMatching       RideStatus = "MATCHING"
	StatusDriverAssigned RideStatus = "DRIVER_ASSIGNED"
	StatusInProgress     RideStatus = "IN_PROGRESS"
	StatusCompleted      RideStatus = "COMPLETED"
	StatusFailed         RideStatus = "FAILED"
)

type Ride struct {
	ID             string
	FareID         string
	PickupLocation Location
	Destination    Location
	EstimatedFare  float64
	Status         RideStatus
	AVID           string
	AVLicensePlate string
}

func (r *Ride) IsActive() bool {
	return r.Status == StatusDriverAssigned || r.Status == StatusInProgress
}

// ─── AV ──────────────────────────────────────────────────────────────────────

type AVStatus string

const (
	AVAvailable AVStatus = "AVAILABLE"
	AVBusy      AVStatus = "BUSY"
	AVOffline   AVStatus = "OFFLINE"
)

type AVLocation struct {
	AVID         string
	Lat          float64
	Lng          float64
	Status       AVStatus
	BatteryLevel float64
}

type DispatchDecision string

const (
	DecisionAccept DispatchDecision = "ACCEPT"
	DecisionReject DispatchDecision = "REJECT"
)

type DispatchCommand struct {
	RideID         string
	PickupLocation Location
	Destination    Location
}

// ─── Domain errors ────────────────────────────────────────────────────────────

var (
	ErrFareNotFound   = errors.New("fare not found")
	ErrRideNotFound   = errors.New("ride not found")
	ErrAVAlreadyBusy  = errors.New("av already assigned to an active ride")
	ErrNoAVAvailable  = errors.New("no available av found nearby")
)
