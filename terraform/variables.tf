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
