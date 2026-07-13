# Echo App — Go + PostgreSQL + Redis + RabbitMQ on EKS

A production-ready Go REST API built with the [Echo](https://echo.labstack.com/) framework, backed by PostgreSQL, Redis, and RabbitMQ. Containerized with Docker, pushed to AWS ECR, and deployed to EKS via a GitHub Actions CI/CD pipeline.

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│   Client     │────▶│  Echo App    │────▶│  PostgreSQL  │
│              │     │  (Go API)    │────▶│  (Database)  │
└─────────────┘     │              │     └──────────────┘
                    │              │────▶┌──────────────┐
                    │              │     │    Redis      │
                    │              │     │   (Cache)     │
                    │              │     └──────────────┘
                    │              │────▶┌──────────────┐
                    │              │     │  RabbitMQ     │
                    └──────────────┘     │  (Messaging)  │
                                        └──────────────┘
```

## Project Structure

```
.
├── cmd/server/main.go          # Application entry point
├── internal/
│   ├── cache/cache.go          # Redis client wrapper
│   ├── config/config.go        # Environment configuration
│   ├── database/database.go    # PostgreSQL with pgx pool
│   ├── handler/handler.go      # Echo HTTP handlers
│   ├── messaging/rabbitmq.go   # RabbitMQ producer & consumer
│   └── model/item.go           # Domain models
├── k8s/                        # Kubernetes manifests
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── postgres.yaml           # DockerHub postgres:16-alpine
│   ├── redis.yaml              # DockerHub redis:7-alpine
│   ├── rabbitmq.yaml           # DockerHub rabbitmq:3-management-alpine
│   └── app.yaml                # App deployment (ECR image) + HPA
├── .github/workflows/
│   └── deploy.yaml             # CI/CD: Test → ECR → EKS
├── Dockerfile                  # Multi-stage build
├── docker-compose.yaml         # Local development
└── go.mod
```

## API Endpoints

| Method | Path              | Description                    |
|--------|-------------------|--------------------------------|
| GET    | `/health`         | Health check (all services)    |
| POST   | `/api/v1/items`   | Create an item                 |
| GET    | `/api/v1/items`   | List all items (cached)        |
| GET    | `/api/v1/items/:id` | Get item by ID (cached)      |
| PUT    | `/api/v1/items/:id` | Update an item               |
| DELETE | `/api/v1/items/:id` | Delete an item               |
| POST   | `/api/v1/events`  | Publish event to RabbitMQ      |

## Local Development

### Prerequisites
- Go 1.23+
- Docker & Docker Compose

### Run locally with Docker Compose

```bash
docker compose up --build
```

The API will be available at `http://localhost:8080`.

### Run Go app directly (requires running infra)

```bash
# Start only infrastructure
docker compose up postgres redis rabbitmq -d

# Run the app
go run ./cmd/server
```

### Example requests

```bash
# Health check
curl http://localhost:8080/health

# Create item
curl -X POST http://localhost:8080/api/v1/items \
  -H "Content-Type: application/json" \
  -d '{"name": "Widget", "description": "A useful widget", "price": 29.99}'

# List items
curl http://localhost:8080/api/v1/items

# Publish event
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"type": "notification", "payload": {"message": "Hello from RabbitMQ!"}}'
```

## CI/CD Pipeline

The GitHub Actions pipeline (`.github/workflows/deploy.yaml`) runs on every push to `main`:

```
┌──────┐     ┌───────────────┐     ┌──────────────┐
│ Test │────▶│ Build & Push  │────▶│ Deploy to    │
│      │     │   to ECR      │     │    EKS       │
└──────┘     └───────────────┘     └──────────────┘
```

### Required GitHub Secrets

| Secret                  | Description                    |
|-------------------------|--------------------------------|
| `AWS_ACCESS_KEY_ID`     | AWS IAM access key             |
| `AWS_SECRET_ACCESS_KEY` | AWS IAM secret key             |

### Required AWS Resources

1. **ECR Repository** — named `echo-app`
2. **EKS Cluster** — named `echo-app-cluster`
3. **IAM User/Role** — with permissions for ECR push and EKS deploy

### Create ECR repository

```bash
aws ecr create-repository --repository-name echo-app --region us-east-1
```

### Create EKS cluster

```bash
eksctl create cluster \
  --name echo-app-cluster \
  --region us-east-1 \
  --nodes 2 \
  --node-type t3.medium
```

## Kubernetes Deployment

Infrastructure services use **DockerHub images**:
- `postgres:16-alpine` — persistent storage with PVC
- `redis:7-alpine` — AOF persistence with LRU eviction
- `rabbitmq:3-management-alpine` — management UI on port 15672

The application image is pulled from **AWS ECR**.

### Manual deployment

```bash
# Configure kubectl
aws eks update-kubeconfig --name echo-app-cluster --region us-east-1

# Deploy everything
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/rabbitmq.yaml

# Replace IMAGE_PLACEHOLDER in app.yaml with your ECR image, then:
kubectl apply -f k8s/app.yaml

# Check status
kubectl -n echo-app get pods
kubectl -n echo-app get svc
```

## Configuration

All configuration is via environment variables:

| Variable       | Default                                                        | Description         |
|----------------|----------------------------------------------------------------|---------------------|
| `PORT`         | `8080`                                                         | Server listen port  |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/echo_app?sslmode=disable` | PostgreSQL DSN |
| `REDIS_URL`    | `redis://localhost:6379/0`                                     | Redis connection    |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/`                           | RabbitMQ AMQP URL   |
