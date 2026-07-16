module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "echo-app-cluster"
  cluster_version = "1.36"

  cluster_endpoint_public_access = true

  vpc_id                   = module.vpc.vpc_id
  subnet_ids               = module.vpc.private_subnets
  control_plane_subnet_ids = module.vpc.private_subnets

  eks_managed_node_groups = {
    app_nodes = {
      instance_types = ["t3.micro"] # Must use t3.micro for AWS Free Tier compliance
      min_size       = 2
      max_size       = 4
      desired_size   = 3 # Increased to 3 nodes to compensate for t3.micro's 4-pod-per-node limit
    }
  }

  # Cluster access entry
  # To add the current caller identity as an administrator
  enable_cluster_creator_admin_permissions = true

  tags = {
    Environment = var.environment
  }
}
