package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AbinterDon/robotaxi-system-design/internal/handler"
	"github.com/AbinterDon/robotaxi-system-design/internal/queue"
	"github.com/AbinterDon/robotaxi-system-design/internal/redisstore"
	"github.com/AbinterDon/robotaxi-system-design/internal/repository"
	"github.com/AbinterDon/robotaxi-system-design/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// ─── Infrastructure ───────────────────────────────────────────────────────
	db := initDB()
	rdb := initRedis()
	store := redisstore.New(rdb)
	q := queue.New(1024)

	// ─── Dependency injection (golang-patterns: avoid global state) ───────────
	fareRepo := repository.NewFareRepo(db)
	rideRepo := repository.NewRideRepo(db)

	fareUC := usecase.NewFareUseCase(fareRepo)
	rideUC := usecase.NewRideUseCase(rideRepo, fareRepo, q)
	matchingUC := usecase.NewMatchingUseCase(rideRepo, store, store, store, q)

	fareH := handler.NewFareHandler(fareUC)
	rideH := handler.NewRideHandler(rideUC)
	avH := handler.NewAVHandler(store, store)

	// ─── Router ───────────────────────────────────────────────────────────────
	r := gin.Default()

	r.POST("/fare", fareH.PostFare)
	r.POST("/rides", rideH.PostRide)
	r.GET("/rides/:id", rideH.GetRide)

	r.POST("/av/location", avH.PostLocation)
	r.GET("/av/:id/dispatch", avH.GetDispatch)
	r.POST("/av/:id/dispatch/:ride_id/decision", avH.PostDecision)

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "healthy"}) })

	// ─── Matching worker (runs in background goroutine) ───────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go matchingUC.Run(ctx)

	// ─── Graceful shutdown (golang-patterns) ─────────────────────────────────
	srv := &http.Server{Addr: ":8080", Handler: r}

	go func() {
		slog.Info("server listening", "addr", ":8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")
	cancel() // stop matching worker

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("forced shutdown", "err", err)
	}
	slog.Info("server exited")
}

func initDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("robotaxi.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		slog.Error("failed to connect to db", "err", err)
		os.Exit(1)
	}
	type fareRecord struct {
		ID                       string  `gorm:"primaryKey"`
		PickupLat, PickupLng     float64
		DestLat, DestLng         float64
		EstimatedFare            float64
		EstimatedDurationMinutes int
		DistanceKm               float64
	}
	type rideRecord struct {
		ID, FareID, Status, AVID, AVLicensePlate string
		PickupLat, PickupLng                      float64
		DestLat, DestLng                          float64
		EstimatedFare                             float64
	}
	if err := db.AutoMigrate(&fareRecord{}, &rideRecord{}); err != nil {
		slog.Error("failed to migrate", "err", err)
		os.Exit(1)
	}
	return db
}

func initRedis() *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	return redis.NewClient(&redis.Options{Addr: addr})
}
