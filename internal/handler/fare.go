// Package handler contains HTTP adapter implementations (gin handlers).
package handler

import (
	"errors"
	"net/http"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"github.com/AbinterDon/robotaxi-system-design/internal/usecase"
	"github.com/gin-gonic/gin"
)

type FareHandler struct {
	uc *usecase.FareUseCase
}

func NewFareHandler(uc *usecase.FareUseCase) *FareHandler {
	return &FareHandler{uc: uc}
}

type fareRequest struct {
	PickupLocation domain.Location `json:"pickup_location" binding:"required"`
	Destination    domain.Location `json:"destination" binding:"required"`
}

// PostFare handles POST /fare
// Step 1: rider inputs pickup/destination and gets a fare estimate.
func (h *FareHandler) PostFare(c *gin.Context) {
	var req fareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fare, err := h.uc.EstimateFare(c.Request.Context(), req.PickupLocation, req.Destination)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"fare_id":                    fare.ID,
		"pickup_location":            fare.PickupLocation,
		"destination":                fare.Destination,
		"estimated_fare":             fare.EstimatedFare,
		"estimated_duration_minutes": fare.EstimatedDurationMinutes,
		"distance_km":                fare.DistanceKm,
	})
}

// ─── Ride handler ─────────────────────────────────────────────────────────────

type RideHandler struct {
	uc *usecase.RideUseCase
}

func NewRideHandler(uc *usecase.RideUseCase) *RideHandler {
	return &RideHandler{uc: uc}
}

type rideRequest struct {
	FareID string `json:"fare_id" binding:"required"`
}

// PostRide handles POST /rides
// Step 2: rider confirms fare and requests a ride.
func (h *RideHandler) PostRide(c *gin.Context) {
	var req rideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ride, err := h.uc.CreateRide(c.Request.Context(), req.FareID)
	if err != nil {
		if errors.Is(err, domain.ErrFareNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "fare not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, toRideResponse(ride))
}

// GetRide handles GET /rides/:id
func (h *RideHandler) GetRide(c *gin.Context) {
	ride, err := h.uc.GetRide(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, domain.ErrRideNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ride not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, toRideResponse(ride))
}

func toRideResponse(r *domain.Ride) gin.H {
	return gin.H{
		"ride_id":         r.ID,
		"status":          r.Status,
		"fare_id":         r.FareID,
		"pickup_location": r.PickupLocation,
		"destination":     r.Destination,
		"estimated_fare":  r.EstimatedFare,
		"av_id":           r.AVID,
		"av_license_plate": r.AVLicensePlate,
	}
}
