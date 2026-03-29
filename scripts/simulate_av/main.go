// AV Fleet Simulator
//
// Simulates a fleet of autonomous vehicles that:
//  1. Send periodic location updates (every 5s, as in the lecture spec)
//  2. Poll for dispatch commands
//  3. Auto-accept (90%) or reject (10%) dispatches
//
// Run alongside the server:
//
//	go run ./scripts/simulate_av
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

const baseURL = "http://localhost:8080"

type av struct {
	id  string
	lat float64
	lng float64
	mu  sync.Mutex
}

var fleet = []*av{
	{id: "av001", lat: 37.7749, lng: -122.4194},
	{id: "av002", lat: 37.7751, lng: -122.4181},
	{id: "av003", lat: 37.7762, lng: -122.4210},
	{id: "av004", lat: 37.7740, lng: -122.4175},
	{id: "av005", lat: 37.7758, lng: -122.4200},
}

func main() {
	slog.Info("starting av fleet simulator", "count", len(fleet))

	var wg sync.WaitGroup
	for _, v := range fleet {
		wg.Add(2)
		go func(v *av) { defer wg.Done(); locationLoop(v) }(v)
		go func(v *av) { defer wg.Done(); dispatchLoop(v) }(v)
	}
	wg.Wait()
}

// locationLoop sends location updates every 5 seconds.
func locationLoop(v *av) {
	for {
		v.mu.Lock()
		v.lat += (rand.Float64()-0.5) * 0.001
		v.lng += (rand.Float64()-0.5) * 0.001
		lat, lng := v.lat, v.lng
		v.mu.Unlock()

		body := map[string]any{
			"av_id":         v.id,
			"lat":           lat,
			"lng":           lng,
			"status":        "AVAILABLE",
			"battery_level": 60 + rand.Float64()*40,
		}
		if err := post("/av/location", body); err != nil {
			slog.Error("location update failed", "av_id", v.id, "err", err)
		}
		time.Sleep(5 * time.Second)
	}
}

// dispatchLoop polls for dispatch commands every second and responds.
func dispatchLoop(v *av) {
	client := &http.Client{Timeout: 5 * time.Second}
	for {
		resp, err := client.Get(baseURL + "/av/" + v.id + "/dispatch")
		if err != nil {
			slog.Error("dispatch poll failed", "av_id", v.id, "err", err)
			time.Sleep(time.Second)
			continue
		}

		var result struct {
			HasCommand bool `json:"has_command"`
			Command    struct {
				RideID string `json:"ride_id"`
			} `json:"command"`
		}
		json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
		resp.Body.Close()

		if result.HasCommand {
			rideID := result.Command.RideID
			decision, reason := "ACCEPT", ""
			if rand.Float64() < 0.1 {
				decision, reason = "REJECT", "LOW_BATTERY"
			}
			slog.Info("dispatch decision", "av_id", v.id, "ride_id", rideID, "decision", decision)

			post(fmt.Sprintf("/av/%s/dispatch/%s/decision", v.id, rideID), map[string]any{ //nolint:errcheck
				"decision": decision,
				"reason":   reason,
			})
		}

		time.Sleep(time.Second)
	}
}

func post(path string, body any) error {
	b, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
