resource "aws_security_group" "mq" {
  name        = "echo-app-rabbitmq-sg"
  description = "Allow access to Amazon MQ RabbitMQ from EKS"
  vpc_id      = module.vpc.vpc_id

  ingress {
    description     = "RabbitMQ AMQPS (TLS) from EKS nodes"
    from_port       = 5671
    to_port         = 5671
    protocol        = "tcp"
    security_groups = [module.eks.node_security_group_id]
  }

  ingress {
    description     = "RabbitMQ Web Console from EKS nodes"
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [module.eks.node_security_group_id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Environment = var.environment
    Name        = "echo-app-rabbitmq-sg"
  }
}

resource "aws_mq_broker" "rabbitmq" {
  broker_name                = "echo-app-rabbitmq"
  engine_type                = "RabbitMQ"
  engine_version             = "3.13"
  host_instance_type         = "mq.m7g.medium"
  security_groups            = [aws_security_group.mq.id]
  subnet_ids                 = [module.vpc.private_subnets[0]] # RabbitMQ single-instance deployment needs 1 subnet ID
  publicly_accessible        = false
  auto_minor_version_upgrade = true

  user {
    username = var.mq_username
    password = var.mq_password
  }

  tags = {
    Environment = var.environment
    Name        = "echo-app-rabbitmq"
  }
}
