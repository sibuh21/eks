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
