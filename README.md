# Echo App — Go + AWS Managed Services (RDS, ElastiCache, Amazon MQ) on EKS

A production-ready Go REST API built with the [Echo](https://echo.labstack.com/) framework, backed by PostgreSQL, Redis, and RabbitMQ. Containerized with Docker, pushed to AWS ECR, and deployed to EKS. 

This branch (`use-aws-services`) utilizes **AWS Managed Services** instead of hosting stateful containers in the EKS cluster:
- **Database**: AWS RDS PostgreSQL
- **Cache**: AWS ElastiCache for Redis
- **Message Broker**: Amazon MQ for RabbitMQ

## Why Move State Out of EKS?

Hosting stateful applications (like database servers) inside EKS with EBS volumes (`gp2`/`gp3`) often encounters mounting deadlocks during rollouts:
1. **`1 old replicas are pending termination` Hanging**: ECR/EKS deployments default to a `RollingUpdate` strategy. EBS volumes (`ReadWriteOnce`) can only mount to a single Node at a time. The new pod cannot start because the volume is locked by the old pod, and the old pod won't terminate until the new pod is ready, creating a deadlock.
2. **`lost+found` Conflicts**: ext4 formatted EBS volumes place a `lost+found/` directory at the mount root. PostgreSQL `initdb` refuses to initialize in a non-empty directory.
3. **Permissions**: EBS volume mounts default to root permissions, causing write failures for non-root containers.

Moving to RDS, ElastiCache, and Amazon MQ eliminates cluster state management, increases reliability, and makes EKS deployments fully stateless and lightweight.

## Architecture

```
                                    ┌──────────────┐
                                ┌──▶│   AWS RDS    │
                                │   │ (PostgreSQL) │
┌─────────────┐     ┌───────────┴──┐└──────────────┘
│   Client     │────▶│  Echo App   │────▶┌──────────────┐
│             │     │ (EKS Pods)   │     │ElastiCache   │
└─────────────┘     └───────────┬──┘     │   (Redis)    │
                                │        └──────────────┘
                                └──▶┌──────────────┐
                                    │  Amazon MQ   │
                                    │  (RabbitMQ)  │
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
│   ├── configmap.yaml          # Non-sensitive config (PORT)
│   ├── secret.yaml             # Sensitive config placeholder (DB, Redis, MQ)
│   └── app.yaml                # App deployment (ECR image) + HPA
├── terraform/                  # Infrastructure as Code
│   ├── main.tf                 # Terraform provider configuration
│   ├── variables.tf            # Variables (VPC, Subnets, Node SG)
│   ├── rds.tf                  # RDS PostgreSQL Setup
│   ├── elasticache.tf          # ElastiCache Redis Setup
│   ├── amazon_mq.tf            # Amazon MQ RabbitMQ Setup
│   └── outputs.tf              # Connection endpoints outputs
├── .github/workflows/
│   └── deploy.yaml             # CI/CD: Test → ECR → EKS + Secret Substitution
├── Dockerfile                  # Multi-stage build
├── docker-compose.yaml         # Local development
└── go.mod
```

## Provisioning AWS Infrastructure (Terraform)

Navigate to the `terraform/` folder and customize variables in `variables.tf` (or create a `terraform.tfvars` file).

```bash
cd terraform
terraform init
terraform apply
```

Specify your existing EKS cluster's VPC ID, subnets, and node security group ID to allow direct communication between the pods and the databases.

## CI/CD Pipeline Configuration

The GitHub Actions pipeline (`.github/workflows/deploy.yaml`) automatically substitutes connection strings from GitHub Secrets into `k8s/secret.yaml` before deploying.

### Required GitHub Secrets

| Secret | Description |
|---|---|
| `AWS_ACCESS_KEY_ID` | AWS access key for deployment |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key for deployment |
| `DATABASE_URL` | `postgres://dbadmin:password@rds-endpoint:5432/echo_app?sslmode=require` |
| `REDIS_URL` | `rediss://elasticache-endpoint:6379` |
| `RABBITMQ_URL` | `amqps://mqadmin:password@amazon-mq-endpoint:5671` |

## Local Development

You can still use Docker Compose for local testing. It spins up local instances of PostgreSQL, Redis, and RabbitMQ:

```bash
docker compose up --build
```
The local API is exposed on `http://localhost:8080`.

