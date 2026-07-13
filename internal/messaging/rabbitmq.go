package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchangeName = "events"
	queueName    = "event_queue"
	routingKey   = "event.#"
)

// RabbitMQ wraps an AMQP connection and channel.
type RabbitMQ struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

// New creates a new RabbitMQ connection, declares the exchange and queue.
func New(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare a topic exchange
	if err := ch.ExchangeDeclare(
		exchangeName,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare queue
	q, err := ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind queue to exchange
	if err := ch.QueueBind(q.Name, routingKey, exchangeName, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	return &RabbitMQ{conn: conn, channel: ch}, nil
}

// Close closes the channel and connection.
func (r *RabbitMQ) Close() {
	if r.channel != nil {
		r.channel.Close()
	}
	if r.conn != nil {
		r.conn.Close()
	}
}

// Ping checks if the RabbitMQ connection is still open.
func (r *RabbitMQ) Ping() error {
	if r.conn.IsClosed() {
		return fmt.Errorf("rabbitmq connection is closed")
	}
	return nil
}

// Publish sends a message to the events exchange.
func (r *RabbitMQ) Publish(ctx context.Context, eventType string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.channel.PublishWithContext(
		ctx,
		exchangeName,
		"event."+eventType,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
}

// StartConsumer starts consuming messages from the event queue.
// This blocks and should be called in a goroutine.
func (r *RabbitMQ) StartConsumer() {
	msgs, err := r.channel.Consume(
		queueName,
		"",    // consumer tag
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		log.Printf("failed to start consumer: %v", err)
		return
	}

	log.Println("RabbitMQ consumer started, waiting for messages...")

	for msg := range msgs {
		log.Printf("Received message [%s]: %s", msg.RoutingKey, string(msg.Body))

		// Process the message (add your business logic here)
		if err := processMessage(msg); err != nil {
			log.Printf("Error processing message: %v", err)
			msg.Nack(false, true) // requeue on failure
			continue
		}

		msg.Ack(false)
	}

	log.Println("RabbitMQ consumer stopped")
}

func processMessage(msg amqp.Delivery) error {
	var event map[string]interface{}
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	log.Printf("Processed event: routing_key=%s payload=%v", msg.RoutingKey, event)
	return nil
}
