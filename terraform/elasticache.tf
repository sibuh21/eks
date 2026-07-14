resource "aws_security_group" "elasticache" {
  name        = "echo-app-redis-sg"
  description = "Allow access to ElastiCache Redis from EKS"
  vpc_id      = var.vpc_id

  ingress {
    description     = "Redis from EKS nodes"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [var.eks_security_group_id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Environment = var.environment
    Name        = "echo-app-redis-sg"
  }
}

resource "aws_elasticache_subnet_group" "redis" {
  name       = "echo-app-redis-subnet-group"
  subnet_ids = var.private_subnet_ids

  tags = {
    Environment = var.environment
    Name        = "echo-app-redis-subnet-group"
  }
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id        = "echo-app-redis"
  description                 = "Redis cluster for echo app"
  node_type                   = "cache.t3.micro"
  num_cache_clusters          = 1
  port                        = 6379
  subnet_group_name           = aws_elasticache_subnet_group.redis.name
  security_group_ids          = [aws_security_group.elasticache.id]
  automatic_failover_enabled  = false
  preferred_cache_cluster_azs = [data.aws_availability_zones.available.names[0]]

  tags = {
    Environment = var.environment
    Name        = "echo-app-redis"
  }
}

data "aws_availability_zones" "available" {
  state = "available"
}
