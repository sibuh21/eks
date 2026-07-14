variable "aws_region" {
  type        = string
  default     = "eu-west-2"
  description = "AWS region for resources"
}

variable "environment" {
  type        = string
  default     = "production"
  description = "Environment name used for tagging"
}

variable "vpc_id" {
  type        = string
  description = "The VPC ID where the EKS cluster is running"
}

variable "private_subnet_ids" {
  type        = list(string)
  description = "List of private subnet IDs for database and cache placement"
}

variable "eks_security_group_id" {
  type        = string
  description = "The security group ID of the EKS node group to allow access to RDS, Redis, and RabbitMQ"
}

variable "db_username" {
  type        = string
  default     = "dbadmin"
  description = "Database administrator username"
}

variable "db_password" {
  type        = string
  sensitive   = true
  description = "Database administrator password"
}

variable "mq_username" {
  type        = string
  default     = "mqadmin"
  description = "RabbitMQ broker administrator username"
}

variable "mq_password" {
  type        = string
  sensitive   = true
  description = "RabbitMQ broker administrator password"
}
