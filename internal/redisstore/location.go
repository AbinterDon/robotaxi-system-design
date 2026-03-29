// Package redisstore implements the LocationStore, DispatchGateway, and
// MatchingStateStore ports using Redis.
package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	geoKey          = "geo:av_locations"
	avStatusPrefix  = "av:status:"
	matchStatePrefix = "match:ride:"
	matchLockPrefix  = "match:lock:"
	dispatchPrefix   = "dispatch:av:"
	decisionPrefix   = "decision:ride:"
	lockTTL          = 15 * time.Second
	stateTTL         = 5 * time.Minute
	decisionTTL      = 60 * time.Second
	decisionPollInterval = 200 * time.Millisecond
	decisionTimeout      = 10 * time.Second
)

type Store struct {
	client *redis.Client
}

func New(client *redis.Client) *Store {
	return &Store{client: client}
}

// ─── LocationStore ────────────────────────────────────────────────────────────

// UpdateLocation stores AV position via GEOADD and metadata in a hash.
// Deep Dive 1: ephemeral location data → Redis, not a relational DB.
func (s *Store) UpdateLocation(ctx context.Context, av domain.AVLocation) error {
	if err := s.client.GeoAdd(ctx, geoKey, &redis.GeoLocation{
		Name:      av.AVID,
		Longitude: av.Lng,
		Latitude:  av.Lat,
	}).Err(); err != nil {
		return fmt.Errorf("geoadd: %w", err)
	}

	return s.client.HSet(ctx, avStatusPrefix+av.AVID,
		"status", string(av.Status),
		"battery_level", av.BatteryLevel,
		"lat", av.Lat,
		"lng", av.Lng,
	).Err()
}

// FindNearbyAvailable uses GEOSEARCH to find AVs within radiusKm, then filters by AVAILABLE status.
func (s *Store) FindNearbyAvailable(ctx context.Context, pickup domain.Location, radiusKm float64, limit int) ([]string, error) {
	locs, err := s.client.GeoSearch(ctx, geoKey, &redis.GeoSearchQuery{
		Longitude:  pickup.Lng,
		Latitude:   pickup.Lat,
		Radius:     radiusKm,
		RadiusUnit: "km",
		Sort:       "ASC",
		Count:      limit * 3, // fetch extra, filter below
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("geosearch: %w", err)
	}

	available := make([]string, 0, limit)
	for _, avID := range locs {
		status, _ := s.client.HGet(ctx, avStatusPrefix+avID, "status").Result()
		if status == string(domain.AVAvailable) {
			available = append(available, avID)
			if len(available) >= limit {
				break
			}
		}
	}
	return available, nil
}

func (s *Store) MarkBusy(ctx context.Context, avID string) error {
	return s.client.HSet(ctx, avStatusPrefix+avID, "status", string(domain.AVBusy)).Err()
}

// ─── DispatchGateway ──────────────────────────────────────────────────────────

func (s *Store) SendCommand(ctx context.Context, avID string, cmd domain.DispatchCommand) error {
	b, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal dispatch command: %w", err)
	}
	return s.client.RPush(ctx, dispatchPrefix+avID, b).Err()
}

func (s *Store) PollCommand(ctx context.Context, avID string) (*domain.DispatchCommand, error) {
	raw, err := s.client.LPop(ctx, dispatchPrefix+avID).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lpop dispatch: %w", err)
	}
	var cmd domain.DispatchCommand
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		return nil, fmt.Errorf("unmarshal dispatch command: %w", err)
	}
	return &cmd, nil
}

func (s *Store) SubmitDecision(ctx context.Context, rideID, avID string, decision domain.DispatchDecision) error {
	key := fmt.Sprintf("%s%s:av:%s", decisionPrefix, rideID, avID)
	pipe := s.client.Pipeline()
	pipe.RPush(ctx, key, string(decision))
	pipe.Expire(ctx, key, decisionTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// WaitDecision polls Redis until the AV's decision arrives or timeout.
func (s *Store) WaitDecision(ctx context.Context, rideID, avID string) (domain.DispatchDecision, error) {
	key := fmt.Sprintf("%s%s:av:%s", decisionPrefix, rideID, avID)
	deadline := time.Now().Add(decisionTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		raw, err := s.client.LPop(ctx, key).Result()
		if err == nil {
			return domain.DispatchDecision(raw), nil
		}
		time.Sleep(decisionPollInterval)
	}
	return domain.DecisionReject, nil // treat timeout as reject
}

// ─── MatchingStateStore ───────────────────────────────────────────────────────

func (s *Store) CreateState(ctx context.Context, rideID string, candidates []string) error {
	b, _ := json.Marshal(candidates)
	key := matchStatePrefix + rideID
	pipe := s.client.Pipeline()
	pipe.HSet(ctx, key, "candidates", string(b), "cursor", 0, "status", "SEARCHING")
	pipe.Expire(ctx, key, stateTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// AcquireLock uses SET NX EX to implement a per-ride distributed lock.
// Deep Dive 3: prevents concurrent workers from dispatching the same ride simultaneously.
func (s *Store) AcquireLock(ctx context.Context, rideID string) (bool, error) {
	ok, err := s.client.SetNX(ctx, matchLockPrefix+rideID, "1", lockTTL).Result()
	return ok, err
}

func (s *Store) ReleaseLock(ctx context.Context, rideID string) error {
	return s.client.Del(ctx, matchLockPrefix+rideID).Err()
}

func (s *Store) GetStatus(ctx context.Context, rideID string) (string, error) {
	return s.client.HGet(ctx, matchStatePrefix+rideID, "status").Result()
}

func (s *Store) MarkDone(ctx context.Context, rideID string) error {
	return s.client.HSet(ctx, matchStatePrefix+rideID, "status", "DONE").Err()
}
