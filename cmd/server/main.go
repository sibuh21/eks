package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/sibuh/eks-echo-app/internal/cache"
	"github.com/sibuh/eks-echo-app/internal/config"
	"github.com/sibuh/eks-echo-app/internal/database"
	"github.com/sibuh/eks-echo-app/internal/handler"
	"github.com/sibuh/eks-echo-app/internal/messaging"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfg := config.Load()

	// Initialize PostgreSQL
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// Initialize Redis
	redisClient, err := cache.New(cfg.RedisURL)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize RabbitMQ
	mq, err := messaging.New(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("failed to connect to rabbitmq: %v", err)
	}
	defer mq.Close()

	// Start consuming messages in the background
	go mq.StartConsumer()

	// Setup Echo
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middleware.RequestID())

	// Initialize handlers
	h := handler.New(db, redisClient, mq)

	// Health check
	e.GET("/health", h.HealthCheck)

	// API routes
	api := e.Group("/api/v1")
	{
		// Items CRUD
		api.POST("/items", h.CreateItem)
		api.GET("/items", h.ListItems)
		api.GET("/items/:id", h.GetItem)
		api.PUT("/items/:id", h.UpdateItem)
		api.DELETE("/items/:id", h.DeleteItem)

		// Events (publishes to RabbitMQ)
		api.POST("/events", h.PublishEvent)
	}

	// Start server
	go func() {
		addr := ":" + cfg.Port
		log.Printf("Starting server on %s", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
	log.Println("Server stopped")
}
