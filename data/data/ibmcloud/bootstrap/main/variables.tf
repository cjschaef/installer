#######################################
# Main Bootstrap module variables
#######################################

variable "bootstrap_dedicated_host_id" {
  type    = string
  default = ""
}

variable "bootstrap_instance_type" {
  type = string
}

variable "bootstrap_subnet_id" {
  type = string
}

variable "bootstrap_subnet_zone" {
  type = string
}

variable "cluster_id" {
  type = string
}

variable "control_plane_security_group_id_list" {
  type = list(string)
}

variable "cos_resource_instance_crn" {
  type = string
}

variable "ibmcloud_region" {
  type = string
}

variable "ignition_bootstrap_file" {
  type = string
}

variable "lb_kubernetes_api_private_id" {
  type = string
}

variable "lb_pool_machine_config_id" {
  type = string
}

variable "public_endpoints" {
  type = bool
}

variable "resource_group_id" {
  type = string
}

variable "tags" {
  type = list(string)
}

variable "vpc_id" {
  type = string
}

variable "vsi_image_id" {
  type = string
}
