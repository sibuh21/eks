package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/sibuh/eks-echo-app/internal/cache"
	"github.com/sibuh/eks-echo-app/internal/database"
	"github.com/sibuh/eks-echo-app/internal/messaging"
	"github.com/sibuh/eks-echo-app/internal/model"

	"github.com/labstack/echo/v4"
)

const itemCacheTTL = 5 * time.Minute

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	db    *database.DB
	cache *cache.Redis
	mq    *messaging.RabbitMQ
}

// New creates a new Handler with all dependencies.
func New(db *database.DB, cache *cache.Redis, mq *messaging.RabbitMQ) *Handler {
	return &Handler{db: db, cache: cache, mq: mq}
}

// HealthCheck returns the health status of all services.
func (h *Handler) HealthCheck(c echo.Context) error {
	ctx := c.Request().Context()

	status := map[string]string{
		"status":   "ok",
		"postgres": "ok",
		"redis":    "ok",
		"rabbitmq": "ok",
	}

	if err := h.db.Ping(ctx); err != nil {
		status["postgres"] = "error: " + err.Error()
		status["status"] = "degraded"
	}

	if err := h.cache.Ping(ctx); err != nil {
		status["redis"] = "error: " + err.Error()
		status["status"] = "degraded"
	}

	if err := h.mq.Ping(); err != nil {
		status["rabbitmq"] = "error: " + err.Error()
		status["status"] = "degraded"
	}

	code := http.StatusOK
	if status["status"] != "ok" {
		code = http.StatusServiceUnavailable
	}

	return c.JSON(code, status)
}

// CreateItem handles POST /api/v1/items
func (h *Handler) CreateItem(c echo.Context) error {
	var item model.Item
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if item.Name == "" || item.Price <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required and price must be positive"})
	}

	ctx := c.Request().Context()
	if err := h.db.CreateItem(ctx, &item); err != nil {
		log.Printf("failed to create item: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create item"})
	}

	// Invalidate list cache
	_ = h.cache.Delete(ctx, "items:list")

	// Publish event
	_ = h.mq.Publish(ctx, "item.created", item)

	return c.JSON(http.StatusCreated, item)
}

// ListItems handles GET /api/v1/items
func (h *Handler) ListItems(c echo.Context) error {
	ctx := c.Request().Context()

	// Try cache first
	var items []model.Item
	found, err := h.cache.Get(ctx, "items:list", &items)
	if err != nil {
		log.Printf("cache get error: %v", err)
	}
	if found {
		return c.JSON(http.StatusOK, items)
	}

	// Fetch from database
	items, err = h.db.ListItems(ctx)
	if err != nil {
		log.Printf("failed to list items: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list items"})
	}

	if items == nil {
		items = []model.Item{}
	}

	// Cache the result
	_ = h.cache.Set(ctx, "items:list", items, itemCacheTTL)

	return c.JSON(http.StatusOK, items)
}

// GetItem handles GET /api/v1/items/:id
func (h *Handler) GetItem(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	ctx := c.Request().Context()
	cacheKey := fmt.Sprintf("items:%d", id)

	// Try cache first
	var item model.Item
	found, err := h.cache.Get(ctx, cacheKey, &item)
	if err != nil {
		log.Printf("cache get error: %v", err)
	}
	if found {
		return c.JSON(http.StatusOK, item)
	}

	// Fetch from database
	result, err := h.db.GetItem(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "item not found"})
	}

	// Cache the result
	_ = h.cache.Set(ctx, cacheKey, result, itemCacheTTL)

	return c.JSON(http.StatusOK, result)
}

// UpdateItem handles PUT /api/v1/items/:id
func (h *Handler) UpdateItem(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	var item model.Item
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if item.Name == "" || item.Price <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required and price must be positive"})
	}

	ctx := c.Request().Context()
	if err := h.db.UpdateItem(ctx, id, &item); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "item not found"})
	}

	// Invalidate caches
	_ = h.cache.Delete(ctx, fmt.Sprintf("items:%d", id))
	_ = h.cache.Delete(ctx, "items:list")

	// Publish event
	_ = h.mq.Publish(ctx, "item.updated", item)

	return c.JSON(http.StatusOK, item)
}

// DeleteItem handles DELETE /api/v1/items/:id
func (h *Handler) DeleteItem(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	ctx := c.Request().Context()
	if err := h.db.DeleteItem(ctx, id); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "item not found"})
	}

	// Invalidate caches
	_ = h.cache.Delete(ctx, fmt.Sprintf("items:%d", id))
	_ = h.cache.Delete(ctx, "items:list")

	// Publish event
	_ = h.mq.Publish(ctx, "item.deleted", map[string]int{"id": id})

	return c.JSON(http.StatusOK, map[string]string{"message": "item deleted"})
}

// PublishEvent handles POST /api/v1/events
func (h *Handler) PublishEvent(c echo.Context) error {
	var event model.Event
	if err := c.Bind(&event); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if event.Type == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "event type is required"})
	}

	ctx := c.Request().Context()
	if err := h.mq.Publish(ctx, event.Type, event.Payload); err != nil {
		log.Printf("failed to publish event: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to publish event"})
	}

	return c.JSON(http.StatusAccepted, map[string]string{"message": "event published"})
}
