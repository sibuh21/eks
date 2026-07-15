# Detailed Walkthrough: EKS + Terraform + AWS Managed Services

This guide explains every detail of how the deployment works, what Terraform created, how resources interact, and the full CI/CD pipeline mechanics.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Terraform: What It Did & How Resources Interact](#2-terraform-what-it-did--how-resources-interact)
3. [EKS Cluster: Scheduling, Pods & Node Limits](#3-eks-cluster-scheduling-pods--node-limits)
4. [Kubernetes Manifests Explained](#4-kubernetes-manifests-explained)
5. [CI/CD Pipeline: Step-by-Step](#5-cicd-pipeline-step-by-step)
6. [Application Code: How It Connects](#6-application-code-how-it-connects)
7. [Common Issues We Solved](#7-common-issues-we-solved)

---

## 1. Architecture Overview

### Before (Self-Hosted in EKS)

```
EKS Cluster
├── postgres Pod (DockerHub image) + PVC (EBS Volume)
├── redis Pod (DockerHub image) + PVC (EBS Volume)
├── rabbitmq Pod (DockerHub image) + PVC (EBS Volume)
└── echo-app Pod (ECR image) ──connects to──▶ above pods via K8s Services
```

**Problems**: EBS volumes lock to one node (ReadWriteOnce), causing deadlocks during rolling updates. PostgreSQL crashes on `lost+found` directories. Containers need specific filesystem permissions (`fsGroup`). All of this creates fragile, hard-to-maintain infrastructure.

### After (AWS Managed Services)

```
AWS Cloud (eu-west-2)
│
├── VPC (vpc-0bb883d9a2bc04e65)
│   │
│   ├── Public Subnets
│   │   └── ELB Load Balancer ──▶ Internet traffic on port 80
│   │
│   ├── Private Subnets (EKS Nodes)
│   │   └── EKS Cluster (echo-app-cluster1)
│   │       └── echo-app Pod ──▶ Reads DATABASE_URL, REDIS_URL, RABBITMQ_URL from K8s Secret
│   │
│   └── Private Subnets (Databases)
│       ├── RDS PostgreSQL (echo-app-db) ──▶ Port 5432
│       ├── ElastiCache Redis (echo-app-redis) ──▶ Port 6379
│       └── Amazon MQ RabbitMQ (echo-app-rabbitmq) ──▶ Port 5671 (TLS)
│
└── Security Groups control which resources can talk to which
```

**Key Insight**: The EKS cluster is now fully **stateless**. It only runs the application container. All state (database rows, cache entries, message queues) lives in AWS-managed services that handle backups, patching, and high availability automatically.

---

## 2. Terraform: What It Did & How Resources Interact

Terraform is a declarative Infrastructure as Code (IaC) tool. You describe the desired end-state of your infrastructure in `.tf` files, and Terraform calculates what needs to be created, modified, or destroyed to reach that state.

### How Terraform Works Internally

```
terraform init     ──▶  Downloads the AWS provider plugin (~674MB binary)
terraform plan     ──▶  Compares your .tf files against terraform.tfstate (current state)
terraform apply    ──▶  Creates/modifies/deletes AWS resources to match your code
terraform destroy  ──▶  Tears down everything Terraform manages
```

The `terraform.tfstate` file is a JSON snapshot of every resource Terraform created. It maps your HCL resource names to real AWS resource IDs. **Never delete this file** or Terraform loses track of what it manages.

### What Each Terraform File Does

#### `main.tf` — Provider Configuration

```hcl
provider "aws" {
  region = var.aws_region   # eu-west-2
}
```

This tells Terraform: "Use the AWS API in the London region. Authenticate using the credentials in my environment (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY)."

#### `variables.tf` — Input Parameters

Defines the inputs Terraform needs:

| Variable | Purpose | Example Value |
|---|---|---|
| `vpc_id` | Which VPC to create resources in | `vpc-0bb883d9a2bc04e65` |
| `private_subnet_ids` | Which subnets for database placement | `["subnet-07ef...", "subnet-046...", "subnet-07b..."]` |
| `eks_security_group_id` | The SG attached to EKS worker nodes | `sg-04cdebcfe3f6c4eae` |
| `db_username` / `db_password` | RDS master credentials | `dbadmin` / `DbPassword12345!` |
| `mq_username` / `mq_password` | Amazon MQ credentials | `mqadmin` / `MqPassword12345!` |

#### `rds.tf` — PostgreSQL Database

Terraform creates **3 resources** for PostgreSQL:

**Resource 1: Security Group** (`aws_security_group.rds`)
```
┌─────────────────────────────────┐
│ echo-app-rds-sg                 │
│                                 │
│ INBOUND:                        │
│   Port 5432 (PostgreSQL)        │
│   FROM: sg-04cdebcfe3f6c4eae    │  ◀── Only EKS nodes can connect
│         (EKS Node SG)           │
│                                 │
│ OUTBOUND:                       │
│   All traffic allowed           │
└─────────────────────────────────┘
```

**Resource 2: DB Subnet Group** (`aws_db_subnet_group.rds`)
- Groups 3 private subnets across different Availability Zones (eu-west-2a, 2b, 2c).
- AWS requires this so it knows which network segments to place the database in.
- Even for single-AZ deployments, AWS needs at least 2 subnets in different AZs.

**Resource 3: RDS Instance** (`aws_db_instance.postgres`)
- Engine: PostgreSQL 16.14
- Instance class: `db.t3.micro` (2 vCPU, 1GB RAM — free tier eligible)
- Storage: 20GB SSD, auto-scaling up to 100GB
- Creates a database named `echo_app` with master user `dbadmin`
- Endpoint output: `echo-app-db.c16kym8ugg0d.eu-west-2.rds.amazonaws.com:5432`

#### `elasticache.tf` — Redis Cache

**Resource 1: Security Group** (`aws_security_group.elasticache`)
- Allows inbound port `6379` only from EKS node security group.

**Resource 2: Subnet Group** (`aws_elasticache_subnet_group.redis`)
- Same private subnets as RDS.

**Resource 3: Replication Group** (`aws_elasticache_replication_group.redis`)
- Single-node Redis cluster on `cache.t3.micro`.
- Endpoint: `echo-app-redis.brrg3e.ng.0001.euw2.cache.amazonaws.com:6379`

#### `amazon_mq.tf` — RabbitMQ Message Broker

**Resource 1: Security Group** (`aws_security_group.mq`)
- Port `5671` (AMQPS / TLS-encrypted AMQP) from EKS nodes.
- Port `443` (RabbitMQ Web Management Console) from EKS nodes.

**Resource 2: MQ Broker** (`aws_mq_broker.rabbitmq`)
- Engine: RabbitMQ 3.13
- Instance: `mq.m7g.medium` (smallest supported for RabbitMQ — `t3.micro` only works for ActiveMQ)
- Single-instance deployment in one private subnet.
- `auto_minor_version_upgrade = true` (required by AWS for RabbitMQ 3.13+).
- Endpoint: `amqps://b-e4b2bef5-ce55-44ed-b760-3a7f992e8d38.mq.eu-west-2.on.aws:5671`

### How Security Groups Create a Trust Chain

```
┌──────────────────┐          ┌──────────────────┐
│  EKS Node SG     │          │  RDS SG          │
│  sg-04cdeb...    │──────────│  sg-08f440...    │
│                  │  Allows  │                  │
│  (attached to    │  port    │  (attached to    │
│   EC2 instances  │  5432    │   RDS instance)  │
│   running pods)  │          │                  │
└──────────────────┘          └──────────────────┘
                    ▲
                    │ Same pattern for Redis (port 6379) and MQ (port 5671)
```

The EKS nodes run with security group `sg-04cdebcfe3f6c4eae`. Each database security group has an **ingress rule** that says: "Allow connections from `sg-04cdebcfe3f6c4eae`." This means only traffic originating from EKS worker nodes can reach the databases. No public internet access.

### `outputs.tf` — Connection Endpoints

After `terraform apply`, these outputs provide the connection strings:

```
rds_endpoint           = "echo-app-db.c16kym8ugg0d.eu-west-2.rds.amazonaws.com:5432"
redis_primary_endpoint = "echo-app-redis.brrg3e.ng.0001.euw2.cache.amazonaws.com"
rabbitmq_amqp_endpoint = "amqps://b-e4b2bef5-ce55-44ed-b760-3a7f992e8d38.mq.eu-west-2.on.aws:5671"
```

These endpoints are then combined with credentials to form full connection URLs stored as GitHub Secrets.

---

## 3. EKS Cluster: Scheduling, Pods & Node Limits

### What is EKS?

Amazon EKS (Elastic Kubernetes Service) is a managed Kubernetes control plane. You provide the **worker nodes** (EC2 instances), and AWS manages the **API server, etcd, and scheduler**.

### Your Cluster Setup

```
echo-app-cluster1
├── Node 1: t3.micro (eu-west-2a) — ip-192-168-60-36
│   ├── aws-node (DaemonSet — VPC CNI plugin)
│   ├── kube-proxy (DaemonSet — network routing)
│   ├── metrics-server (Deployment — resource monitoring)
│   └── [1 SLOT FREE for app pods]
│
└── Node 2: t3.micro (eu-west-2c) — ip-192-168-71-236
    ├── aws-node (DaemonSet)
    ├── kube-proxy (DaemonSet)
    ├── coredns (Deployment — DNS resolution)
    └── [1 SLOT FREE for app pods]
```

### The t3.micro Pod Limit Problem

Every pod in EKS gets a real VPC IP address (via the AWS VPC CNI plugin). The number of IPs depends on how many Elastic Network Interfaces (ENIs) the instance type supports:

| Instance Type | Max ENIs | IPv4 per ENI | Max Pods |
|---|---|---|---|
| `t3.micro` | 2 | 2 | **4** |
| `t3.small` | 3 | 4 | **11** |
| `t3.medium` | 3 | 6 | **17** |

With only **4 pod slots per node** and 2 nodes, you have a maximum of **8 pods cluster-wide**. System DaemonSets (`aws-node`, `kube-proxy`) consume 2 slots per node automatically, leaving only **2 free slots per node**.

### The Memory Budget

```
t3.micro total RAM:        933 MiB
AWS system reservation:   -408 MiB
──────────────────────────────────
Allocatable for pods:      525 MiB
```

If `metrics-server` requests 200MiB, that leaves only ~325MiB for your app. This is why we reduced the app's memory request to `64Mi`.

---

## 4. Kubernetes Manifests Explained

### `k8s/namespace.yaml` — Logical Isolation

Creates the `echo-app` namespace. Namespaces isolate resources (pods, services, secrets) from other workloads in the same cluster.

### `k8s/configmap.yaml` — Non-Sensitive Configuration

```yaml
data:
  PORT: "8080"
```

Stores non-sensitive key-value pairs. Injected into the pod as environment variables via `envFrom.configMapRef`.

### `k8s/app.yaml` — The Application Deployment

**Deployment** (lines 1-53):
- `replicas: 1` — Run one instance of the app (constrained by t3.micro capacity).
- `strategy.rollingUpdate.maxUnavailable: 1, maxSurge: 0` — During updates, kill the old pod first, then start the new one. This avoids needing two pods simultaneously (which would exceed node capacity).
- `envFrom` loads both the ConfigMap (PORT) and Secret (DATABASE_URL, REDIS_URL, RABBITMQ_URL) as environment variables.
- Resource requests (`20m` CPU, `64Mi` memory) are intentionally small to fit on t3.micro nodes.
- `readinessProbe` and `livenessProbe` hit `/health` to verify the app can reach all three backends.

**Service** (lines 55-70):
- `type: LoadBalancer` — AWS automatically provisions a Classic/Network Load Balancer with a public DNS name.
- Maps external port `80` to container port `8080`.

### How the Secret Gets Created (No YAML File)

Instead of a static `secret.yaml` file, the CI/CD pipeline creates the secret dynamically:

```bash
kubectl create secret generic echo-app-secret \
  --from-literal=DATABASE_URL="$DATABASE_URL" \
  --from-literal=REDIS_URL="$REDIS_URL" \
  --from-literal=RABBITMQ_URL="$RABBITMQ_URL" \
  --dry-run=client -o yaml | kubectl apply -f -
```

This approach:
- **Avoids committing secrets to Git** (security best practice).
- **Handles special characters** in passwords (e.g., `!`, `@`, `?`) without breaking `sed` or YAML parsing.
- `--dry-run=client -o yaml` generates the Secret manifest in memory without contacting the cluster.
- `kubectl apply -f -` reads from stdin and applies it idempotently (creates or updates).

---

## 5. CI/CD Pipeline: Step-by-Step

The pipeline is defined in `.github/workflows/deploy.yaml` and triggers on every push to the `main` branch.

### Job 1: `test` — Code Quality

```yaml
- uses: actions/setup-go@v5         # Install Go 1.23
- run: go test -v -race ./...       # Run tests with race detector
```

If tests fail, the entire pipeline stops. No broken code gets deployed.

### Job 2: `build-and-push` — Container Image

```yaml
- uses: aws-actions/amazon-ecr-login  # Authenticate Docker to ECR
- run: docker build -t $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG .
- run: docker push $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG
```

The Dockerfile uses a **multi-stage build**:
1. **Stage 1 (Builder)**: Full Go SDK image, compiles the binary.
2. **Stage 2 (Runner)**: Minimal `scratch` or Alpine image, copies only the compiled binary.

Result: A ~8.5MB container image (vs ~1GB for a full Go SDK image).

The image is tagged with the Git commit SHA (e.g., `f74082ca101a64e2601ce51fe2899afad72124e5`), ensuring every deployment is traceable to an exact code version.

### Job 3: `deploy` — Kubernetes Rollout

**Step 1: Authenticate to EKS**
```yaml
- run: aws eks update-kubeconfig --name $EKS_CLUSTER_NAME --region $AWS_REGION
```
Downloads the cluster's CA certificate and API endpoint into `~/.kube/config`, enabling `kubectl` commands.

**Step 2: Deploy Secrets & ConfigMaps**
```yaml
- run: |
    kubectl apply -f k8s/namespace.yaml
    kubectl apply -f k8s/configmap.yaml
    kubectl create secret generic echo-app-secret \
      --from-literal=DATABASE_URL="$DATABASE_URL" \
      ... --dry-run=client -o yaml | kubectl apply -f -
```

**Step 3: Deploy the Application**
```yaml
- run: |
    sed -i "s|IMAGE_PLACEHOLDER|$IMAGE|g" k8s/app.yaml
    kubectl apply -f k8s/app.yaml
```
Replaces `IMAGE_PLACEHOLDER` in app.yaml with the real ECR image URI, then applies the manifest.

**Step 4: Wait for Rollout**
```yaml
- run: kubectl -n $K8S_NAMESPACE rollout status deployment/echo-app --timeout=180s
```
Blocks until the new pod is `Ready` (passes health checks) or times out after 3 minutes.

---

## 6. Application Code: How It Connects

### Config Loading (`internal/config/config.go`)

```go
func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return strings.TrimSpace(v)  // Critical: removes trailing \n from secrets
    }
    return fallback
}
```

`strings.TrimSpace()` is essential because GitHub Actions sometimes injects trailing newlines into secret values. Without trimming, the URL parser sees `\n` as an "invalid control character" and crashes the app.

### Database Connection (`internal/database/database.go`)

```go
pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
```

`pgxpool` parses the `DATABASE_URL` (e.g., `postgres://dbadmin:pass@rds-endpoint:5432/echo_app?sslmode=require`) and creates a connection pool to RDS. The `?sslmode=require` parameter forces TLS encryption in transit.

### Redis Connection (`internal/cache/cache.go`)

```go
opt, _ := redis.ParseURL(cfg.RedisURL)
client := redis.NewClient(opt)
```

Parses `redis://elasticache-endpoint:6379/0` and establishes a TCP connection to ElastiCache.

### RabbitMQ Connection (`internal/messaging/rabbitmq.go`)

```go
conn, err := amqp.Dial(cfg.RabbitMQURL)
```

Connects to `amqps://user:pass@amazon-mq-endpoint:5671` using TLS (the `amqps://` scheme). Amazon MQ for RabbitMQ only supports TLS connections on port 5671.

---

## 7. Common Issues We Solved

### Issue 1: EBS Volume Deadlock (Before Migration)

**Symptom**: `1 old replicas are pending termination` during rolling updates.

**Root Cause**: EBS volumes use `ReadWriteOnce` access mode. During a rolling update, the new pod tries to mount the volume, but it's locked by the old pod. The old pod won't terminate until the new pod is ready. Deadlock.

**Solution**: Migrated to RDS/ElastiCache/Amazon MQ, eliminating all PersistentVolumeClaims from the EKS cluster.

### Issue 2: PostgreSQL `lost+found` Crash

**Symptom**: PostgreSQL container enters `CrashLoopBackOff` with `initdb: directory "/var/lib/postgresql/data" is not empty`.

**Root Cause**: EBS volumes are ext4-formatted and always contain a `lost+found` directory at the root. PostgreSQL refuses to initialize in a non-empty directory.

**Previous Workaround**: Set `PGDATA=/var/lib/postgresql/data/pgdata` (subdirectory).

**Permanent Fix**: Use RDS — no filesystem management needed.

### Issue 3: `sed` Parsing Failure with Special Characters

**Symptom**: `sed: -e expression #1, char 145: unterminated 's' command`

**Root Cause**: Connection strings contain characters like `?`, `/`, `!` that break `sed` delimiter parsing. GitHub Secrets also inject trailing newlines.

**Solution**: Replaced `sed`-based substitution with `kubectl create secret --from-literal`, which handles any characters safely.

### Issue 4: Node Capacity Exhaustion

**Symptom**: All pods stuck in `Pending` with `0/2 nodes are available: Too many pods`.

**Root Cause**: `t3.micro` instances only support 4 pods per node. System pods consumed all slots.

**Solution**: Scaled down system deployments, removed self-hosted database pods, reduced app replicas to 1, and minimized resource requests.

### Issue 5: Terraform Version/Instance Type Errors

**Symptom**: `Cannot find version 16.3 for postgres` and `does not support host instance type mq.t3.micro`.

**Root Cause**: PostgreSQL 16.3 was deprecated in eu-west-2. RabbitMQ on Amazon MQ doesn't support `t3` instance families.

**Solution**: Queried AWS APIs to find valid versions (`16.14`) and instance types (`mq.m7g.medium`).

---

## Quick Reference: Full Deployment Commands

```bash
# 1. Provision AWS infrastructure
cd terraform
cp terraform.tfvars.example terraform.tfvars   # Edit with your values
terraform init
terraform apply

# 2. Note the outputs
terraform output

# 3. Set GitHub Secrets (in GitHub UI: Settings > Secrets > Actions)
#    DATABASE_URL = postgres://dbadmin:pass@<rds_endpoint>/echo_app?sslmode=require
#    REDIS_URL    = redis://<redis_endpoint>:6379/0
#    RABBITMQ_URL = amqps://mqadmin:pass@<mq_endpoint>:5671

# 4. Push code to main branch — CI/CD handles the rest
git push origin main

# 5. Verify deployment
kubectl -n echo-app get pods
curl http://<loadbalancer-url>/health
```
