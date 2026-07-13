package model

import "time"

// Item represents a resource stored in PostgreSQL and cached in Redis.
type Item struct {
	ID          int       `json:"id"`
	Name        string    `json:"name" validate:"required"`
	Description string    `json:"description"`
	Price       float64   `json:"price" validate:"required,gt=0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Event represents a message published to RabbitMQ.
type Event struct {
	Type    string      `json:"type" validate:"required"`
	Payload interface{} `json:"payload" validate:"required"`
}
