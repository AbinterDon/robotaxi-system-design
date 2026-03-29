package usecase

import (
	"context"
	"fmt"
	"math"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"github.com/google/uuid"
)

const (
	baseFareUSD    = 2.0
	pricePerKmUSD  = 1.5
	avgSpeedKmh    = 40.0
	pickupBufferMin = 5
)

type FareUseCase struct {
	repo domain.FareRepository
}

func NewFareUseCase(repo domain.FareRepository) *FareUseCase {
	return &FareUseCase{repo: repo}
}

func (uc *FareUseCase) EstimateFare(ctx context.Context, pickup, dest domain.Location) (*domain.Fare, error) {
	distKm := haversineKm(pickup.Lat, pickup.Lng, dest.Lat, dest.Lng)
	fareAmt := math.Round((baseFareUSD+pricePerKmUSD*distKm)*100) / 100
	durationMin := int(distKm/avgSpeedKmh*60) + pickupBufferMin

	fare := &domain.Fare{
		ID:                       uuid.NewString(),
		PickupLocation:           pickup,
		Destination:              dest,
		EstimatedFare:            fareAmt,
		EstimatedDurationMinutes: durationMin,
		DistanceKm:               math.Round(distKm*100) / 100,
	}

	if err := uc.repo.Save(ctx, fare); err != nil {
		return nil, fmt.Errorf("estimate fare: %w", err)
	}
	return fare, nil
}

// haversineKm returns the great-circle distance in kilometers.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0
	dlat := toRad(lat2 - lat1)
	dlng := toRad(lng2 - lng1)
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dlng/2)*math.Sin(dlng/2)
	return R * 2 * math.Asin(math.Sqrt(a))
}

func toRad(deg float64) float64 { return deg * math.Pi / 180 }
