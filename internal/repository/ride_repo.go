package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"gorm.io/gorm"
)

// rideRecord is the GORM DB model for rides.
type rideRecord struct {
	ID             string `gorm:"primaryKey"`
	FareID         string
	PickupLat      float64
	PickupLng      float64
	DestLat        float64
	DestLng        float64
	EstimatedFare  float64
	Status         string
	AVID           string
	AVLicensePlate string
}

func (rideRecord) TableName() string { return "rides" }

type RideRepo struct {
	db *gorm.DB
}

func NewRideRepo(db *gorm.DB) *RideRepo {
	return &RideRepo{db: db}
}

func (r *RideRepo) Save(ctx context.Context, ride *domain.Ride) error {
	rec := toRideRecord(ride)
	if err := r.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return fmt.Errorf("ride repo save: %w", err)
	}
	return nil
}

func (r *RideRepo) FindByID(ctx context.Context, id string) (*domain.Ride, error) {
	var rec rideRecord
	err := r.db.WithContext(ctx).First(&rec, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrRideNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ride repo find: %w", err)
	}
	return toRideDomain(&rec), nil
}

func (r *RideRepo) UpdateStatus(ctx context.Context, id string, status domain.RideStatus) error {
	res := r.db.WithContext(ctx).Model(&rideRecord{}).Where("id = ?", id).Update("status", string(status))
	if res.Error != nil {
		return fmt.Errorf("ride repo update status: %w", res.Error)
	}
	return nil
}

// AssignAV sets the AV on a ride.
// Deep Dive 4: Checks that the AV has no other active ride before assigning.
// In production PostgreSQL this is enforced by:
//
//	CREATE UNIQUE INDEX uniq_active_ride_per_av ON rides(av_id)
//	WHERE status IN ('DRIVER_ASSIGNED', 'IN_PROGRESS');
func (r *RideRepo) AssignAV(ctx context.Context, rideID, avID, licensePlate string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check for existing active ride on this AV
		var count int64
		tx.Model(&rideRecord{}).
			Where("av_id = ? AND status IN ?", avID, []string{
				string(domain.StatusDriverAssigned),
				string(domain.StatusInProgress),
			}).Count(&count)
		if count > 0 {
			return domain.ErrAVAlreadyBusy
		}

		res := tx.Model(&rideRecord{}).Where("id = ? AND status = ?", rideID, string(domain.StatusMatching)).
			Updates(map[string]any{
				"av_id":            avID,
				"av_license_plate": licensePlate,
				"status":           string(domain.StatusDriverAssigned),
			})
		if res.Error != nil {
			return fmt.Errorf("assign av: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return domain.ErrRideNotFound
		}
		return nil
	})
}

func toRideRecord(ride *domain.Ride) rideRecord {
	return rideRecord{
		ID:             ride.ID,
		FareID:         ride.FareID,
		PickupLat:      ride.PickupLocation.Lat,
		PickupLng:      ride.PickupLocation.Lng,
		DestLat:        ride.Destination.Lat,
		DestLng:        ride.Destination.Lng,
		EstimatedFare:  ride.EstimatedFare,
		Status:         string(ride.Status),
		AVID:           ride.AVID,
		AVLicensePlate: ride.AVLicensePlate,
	}
}

func toRideDomain(r *rideRecord) *domain.Ride {
	return &domain.Ride{
		ID:             r.ID,
		FareID:         r.FareID,
		PickupLocation: domain.Location{Lat: r.PickupLat, Lng: r.PickupLng},
		Destination:    domain.Location{Lat: r.DestLat, Lng: r.DestLng},
		EstimatedFare:  r.EstimatedFare,
		Status:         domain.RideStatus(r.Status),
		AVID:           r.AVID,
		AVLicensePlate: r.AVLicensePlate,
	}
}
