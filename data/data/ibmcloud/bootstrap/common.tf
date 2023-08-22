locals {
  description      = "Created By OpenShift Installer"
  public_endpoints = var.ibmcloud_publish_strategy == "External" ? true : false
  tags = concat(
    ["kubernetes.io_cluster_${var.cluster_id}:owned"],
    var.ibmcloud_extra_tags
  )
}

############################################
# IBM Cloud provider
############################################

provider "ibm" {
  ibmcloud_api_key = var.ibmcloud_api_key
  region           = var.ibmcloud_region

  # Manage endpoints for IBM Cloud services
  visibility          = var.ibmcloud_publish_strategy == "External" ? "public" : "private"
  endpoints_file_path = var.ibmcloud_service_endpoints_json
}
