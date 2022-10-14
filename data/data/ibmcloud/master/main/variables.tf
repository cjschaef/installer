#######################################
# Master Main module variables
#######################################

variable "cluster_id" {
  type = string
}

variable "control_plane_count" {
  type = string
}

variable "control_plane_dedicated_host_id_list" {
  type = list(string)
}

variable "control_plane_ignition" {
  type = string
}

variable "control_plane_instance_type" {
  type = string
}

variable "control_plane_security_group_id_list" {
  type = list(string)
}

variable "control_plane_subnet_id_list" {
  type = list(string)
}

variable "control_plane_subnet_zone_list" {
  type = list(string)
}

variable "lb_kubernetes_api_private_id" {
  type = string
}

variable "lb_pool_machine_config_id" {
  type = string
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
