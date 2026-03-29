package handler

import (
	"net/http"

	"github.com/AbinterDon/robotaxi-system-design/internal/domain"
	"github.com/gin-gonic/gin"
)

// AVHandler handles AV endpoints. In production these would be gRPC streams;
// here simplified to HTTP polling.
type AVHandler struct {
	locationStore domain.LocationStore
	dispatch      domain.DispatchGateway
}

func NewAVHandler(locationStore domain.LocationStore, dispatch domain.DispatchGateway) *AVHandler {
	return &AVHandler{locationStore: locationStore, dispatch: dispatch}
}

type avLocationRequest struct {
	AVID         string          `json:"av_id" binding:"required"`
	Lat          float64         `json:"lat" binding:"required"`
	Lng          float64         `json:"lng" binding:"required"`
	Status       domain.AVStatus `json:"status"`
	BatteryLevel float64         `json:"battery_level"`
}

// PostLocation handles POST /av/location
// Deep Dive 1: AV sends periodic location updates (every ~5s in production).
func (h *AVHandler) PostLocation(c *gin.Context) {
	var req avLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status == "" {
		req.Status = domain.AVAvailable
	}
	if req.BatteryLevel == 0 {
		req.BatteryLevel = 100
	}

	err := h.locationStore.UpdateLocation(c.Request.Context(), domain.AVLocation{
		AVID:         req.AVID,
		Lat:          req.Lat,
		Lng:          req.Lng,
		Status:       req.Status,
		BatteryLevel: req.BatteryLevel,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetDispatch handles GET /av/:id/dispatch
// AV polls for a pending dispatch command (simplified from gRPC DispatchCommand).
func (h *AVHandler) GetDispatch(c *gin.Context) {
	avID := c.Param("id")
	cmd, err := h.dispatch.PollCommand(c.Request.Context(), avID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cmd == nil {
		c.JSON(http.StatusOK, gin.H{"has_command": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"has_command": true, "command": cmd})
}

type dispatchDecisionRequest struct {
	Decision domain.DispatchDecision `json:"decision" binding:"required"`
	Reason   string                  `json:"reason"`
}

// PostDecision handles POST /av/:id/dispatch/:ride_id/decision
// AV responds with ACCEPT or REJECT (simplified from gRPC DispatchDecision).
func (h *AVHandler) PostDecision(c *gin.Context) {
	var req dispatchDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.dispatch.SubmitDecision(
		c.Request.Context(),
		c.Param("ride_id"),
		c.Param("id"),
		req.Decision,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
