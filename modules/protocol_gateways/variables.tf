variable "rg_name" {
  type        = string
  description = "A predefined resource group in the Azure subscription."
}

variable "instance_type" {
  type        = string
  description = "The virtual machine type (sku) to deploy."
}

variable "protocol" {
  type        = string
  description = "Name of the protocol."
  default     = "NFS"

  validation {
    condition     = contains(["NFS", "SMB"], var.protocol)
    error_message = "Allowed values for protocol: NFS, SMB."
  }
}

variable "secondary_ips_per_nic" {
  type        = number
  description = "Number of secondary IPs per single NIC per protocol gateway virtual machine."
  default     = 1
}

variable "vnet_rg_name" {
  type        = string
  description = "Resource group name of vnet"
}

variable "vnet_name" {
  type        = string
  description = "The virtual network name."
}

variable "subnet_name" {
  type        = string
  description = "The subnet names."
}

variable "tags_map" {
  type        = map(string)
  default     = {}
  description = "A map of tags to assign the same metadata to all resources in the environment. Format: key:value."
}

variable "gateways_number" {
  type        = number
  description = "The number of virtual machines to deploy as protocol gateways."
}

variable "gateways_name" {
  type        = string
  description = "The protocol group name."
}

variable "interface_group_name" {
  type        = string
  description = "Interface group name."
  default     = "weka-ig"

  validation {
    condition     = length(var.interface_group_name) <= 11
    error_message = "The interface group name should be up to 11 characters long."
  }
}

variable "client_group_name" {
  type        = string
  description = "Client access group name."
  default     = "weka-cg"
}

variable "backend_lb_ip" {
  type        = string
  description = "The backend load balancer ip address."
}

variable "vm_username" {
  type        = string
  description = "The user name for logging in to the virtual machines."
  default     = "weka"
}

variable "nics" {
  type        = number
  default     = 2
  description = "Number of nics to set on each vm"

  validation {
    condition     = var.nics >= 2
    error_message = "The amount of NICs per protocol gateway VM should be at least 2."
  }
}

variable "ssh_public_key" {
  type        = string
  description = "The VM public key. If it is not set, the keys are auto-generated."
}

variable "assign_public_ip" {
  type        = bool
  default     = true
  description = "Determines whether to assign public ip."
}

variable "ppg_id" {
  type        = string
  description = "Placement proximity group id."
}

variable "sg_id" {
  type        = string
  description = "Security group id."
}

variable "source_image_id" {
  type        = string
  description = "Use weka custom image, ubuntu 20.04 with kernel 5.4 and ofed 5.8-1.1.2.1"
}

variable "apt_repo_url" {
  type        = string
  default     = ""
  description = "The URL of the apt private repository."
}

variable "disk_size" {
  type        = number
  description = "The disk size."
}

variable "traces_per_frontend" {
  default     = 10
  type        = number
  description = "The number of traces per frontend ionode. Traces are low-level events generated by Weka processes and are used as troubleshooting information for support purposes. Protocol gateways have only frontend ionodes."
}

variable "frontend_num" {
  type        = number
  default     = 1
  description = "The number of frontend ionodes per instance."
}

variable "install_weka_url" {
  type        = string
  description = "The URL of the Weka release download tar file."

  validation {
    condition     = length(var.install_weka_url) > 0
    error_message = "The URL should not be empty."
  }
}

variable "key_vault_url" {
  type        = string
  description = "The URL of the Azure Key Vault."
}
