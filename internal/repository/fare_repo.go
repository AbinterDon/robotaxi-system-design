// Package repository contains database adapter implementations.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"gorm.io/gorm"
)

// fareRecord is the GORM DB model for fares.
type fareRecord struct {
	ID                       string  `gorm:"primaryKey"`
	PickupLat                float64
	PickupLng                float64
	DestLat                  float64
	DestLng                  float64
	EstimatedFare            float64
	EstimatedDurationMinutes int
	DistanceKm               float64
}

func (fareRecord) TableName() string { return "fares" }

type FareRepo struct {
	db *gorm.DB
}

func NewFareRepo(db *gorm.DB) *FareRepo {
	return &FareRepo{db: db}
}

func (r *FareRepo) Save(ctx context.Context, fare *domain.Fare) error {
	rec := toFareRecord(fare)
	if err := r.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return fmt.Errorf("fare repo save: %w", err)
	}
	return nil
}

func (r *FareRepo) FindByID(ctx context.Context, id string) (*domain.Fare, error) {
	var rec fareRecord
	err := r.db.WithContext(ctx).First(&rec, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrFareNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fare repo find: %w", err)
	}
	return toFareDomain(&rec), nil
}

func toFareRecord(f *domain.Fare) fareRecord {
	return fareRecord{
		ID:                       f.ID,
		PickupLat:                f.PickupLocation.Lat,
		PickupLng:                f.PickupLocation.Lng,
		DestLat:                  f.Destination.Lat,
		DestLng:                  f.Destination.Lng,
		EstimatedFare:            f.EstimatedFare,
		EstimatedDurationMinutes: f.EstimatedDurationMinutes,
		DistanceKm:               f.DistanceKm,
	}
}

func toFareDomain(r *fareRecord) *domain.Fare {
	return &domain.Fare{
		ID:                       r.ID,
		PickupLocation:           domain.Location{Lat: r.PickupLat, Lng: r.PickupLng},
		Destination:              domain.Location{Lat: r.DestLat, Lng: r.DestLng},
		EstimatedFare:            r.EstimatedFare,
		EstimatedDurationMinutes: r.EstimatedDurationMinutes,
		DistanceKm:               r.DistanceKm,
	}
}
