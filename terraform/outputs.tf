output "rds_endpoint" {
  description = "RDS PostgreSQL connection endpoint"
  value       = aws_db_instance.postgres.endpoint
}

output "redis_primary_endpoint" {
  description = "Redis replication group primary endpoint"
  value       = aws_elasticache_replication_group.redis.primary_endpoint_address
}

output "rabbitmq_amqp_endpoint" {
  description = "RabbitMQ broker endpoints"
  value       = aws_mq_broker.rabbitmq.instances[0].endpoints[0] # SSL AMQP endpoint (amqps://)
}

output "eks_cluster_endpoint" {
  description = "Endpoint for EKS control plane"
  value       = module.eks.cluster_endpoint
}

output "eks_cluster_name" {
  description = "Kubernetes Cluster Name"
  value       = module.eks.cluster_name
}

output "ecr_repository_url" {
  description = "URL of the ECR repository"
  value       = aws_ecr_repository.app.repository_url
}
