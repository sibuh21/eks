resource "aws_security_group" "rds" {
  name        = "echo-app-rds-sg"
  description = "Allow access to RDS PostgreSQL from EKS"
  vpc_id      = var.vpc_id

  ingress {
    description     = "PostgreSQL from EKS nodes"
    from_port       = 5432
    to_port         = 5432
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
    Name        = "echo-app-rds-sg"
  }
}

resource "aws_db_subnet_group" "rds" {
  name       = "echo-app-rds-subnet-group"
  subnet_ids = var.private_subnet_ids

  tags = {
    Environment = var.environment
    Name        = "echo-app-rds-subnet-group"
  }
}

resource "aws_db_instance" "postgres" {
  identifier             = "echo-app-db"
  engine                 = "postgres"
  engine_version         = "16.14"
  instance_class         = "db.t3.micro"
  allocated_storage      = 20
  max_allocated_storage  = 100
  db_name                = "echo_app"
  username               = var.db_username
  password               = var.db_password
  db_subnet_group_name   = aws_db_subnet_group.rds.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  skip_final_snapshot    = true
  publicly_accessible    = false

  tags = {
    Environment = var.environment
    Name        = "echo-app-postgres"
  }
}
