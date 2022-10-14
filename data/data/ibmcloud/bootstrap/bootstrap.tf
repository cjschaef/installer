############################################
# Main Bootstrap Module
############################################

module "main" {
  source = "./main"

  cluster_id        = var.cluster_id
  resource_group_id = var.resource_group_id
  vpc_id            = var.vpc_id
  vsi_image_id      = var.vsi_image_id
  tags              = local.tags

  bootstrap_instance_type     = var.ibmcloud_bootstrap_instance_type
  bootstrap_subnet_id         = var.control_plane_subnet_id_list[0]
  bootstrap_subnet_zone       = var.control_plane_subnet_zone_list[0]
  bootstrap_dedicated_host_id = length(var.control_plane_dedicated_host_id_list) > 0 ? var.control_plane_dedicated_host_id_list[0] : null

  control_plane_security_group_id_list = var.control_plane_security_group_id_list

  cos_resource_instance_crn = var.cos_resource_instance_crn
  ibmcloud_region           = var.ibmcloud_region
  ignition_bootstrap_file   = var.ignition_bootstrap_file

  lb_kubernetes_api_private_id = var.lb_kubernetes_api_private_id
  lb_pool_machine_config_id    = var.lb_pool_machine_config_id

  public_endpoints = local.public_endpoints
}


############################################
# LB Backend Pool Module
############################################
# Remaining LB backend pool members are configured separately
# due to timing windows for IBM Cloud ALB's in relation to
# https://issues.redhat.com/browse/OCPBUGS-1327

module "lb_pools" {
  source     = "./lb_pools"
  depends_on = [module.main]

  lb_kubernetes_api_public_id       = var.lb_kubernetes_api_public_id
  lb_pool_kubernetes_api_public_id  = var.lb_pool_kubernetes_api_public_id
  lb_kubernetes_api_private_id      = var.lb_kubernetes_api_private_id
  lb_pool_kubernetes_api_private_id = var.lb_pool_kubernetes_api_private_id

  bootstrap_node_ip = module.main.bootstrap_ip

  public_endpoints = local.public_endpoints
}
