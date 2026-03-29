// Package queue provides an in-memory async queue.
// In production this would be replaced by Kafka / AWS SQS.
//
// Deep Dive 2: The queue decouples Ride Service (producer) from Matching Service
// (consumer), absorbing traffic spikes without dropping requests.
package queue

import (
	"context"
	"time"
)

const consumeTimeout = 1 * time.Second

type RideQueue struct {
	ch chan string
}

func New(bufferSize int) *RideQueue {
	return &RideQueue{ch: make(chan string, bufferSize)}
}

func (q *RideQueue) Publish(_ context.Context, rideID string) error {
	q.ch <- rideID
	return nil
}

func (q *RideQueue) Consume(ctx context.Context) (string, error) {
	select {
	case rideID := <-q.ch:
		return rideID, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(consumeTimeout):
		return "", nil // no-op timeout, caller re-loops
	}
}
