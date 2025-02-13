variable "prefix" {
  type        = string
  description = "The prefix for all the resource names. For example, the prefix for your system name."
  default     = "weka"
}

variable "rg_name" {
  type        = string
  description = "A predefined resource group in the Azure subscription."
}

variable "address_space" {
  type        = string
  description = "The range of IP addresses the virtual network uses."
  default     = "10.0.0.0/16"
}

variable "subnet_prefix" {
  type        = string
  description = "Address prefixes to use for the subnet."
  default     = "10.0.2.0/24"
}

variable "tags_map" {
  type        = map(string)
  default     = { "env" : "dev", "creator" : "tf" }
  description = "A map of tags to assign the same metadata to all resources in the environment. Format: key:value."
}

variable "vnet_name" {
  type        = string
  default     = ""
  description = "The VNet name, if exists."
}

variable "subnet_name" {
  type        = string
  default     = ""
  description = "Subnet name, if exist."
}

variable "allow_ssh_cidrs" {
  type        = list(string)
  description = "Allow port 22, if not provided, i.e leaving the default empty list, the rule will not be included in the SG"
  default     = []
}

variable "allow_weka_api_cidrs" {
  type        = list(string)
  description = "Allow connection to port 14000 on weka backends from specified CIDRs, by default no CIDRs are allowed. All ports (including 14000) are allowed within Vnet"
  default     = []
}

variable "sg_custom_ingress_rules" {
  type = list(object({
    from_port  = string
    to_port    = string
    protocol   = string
    cidr_block = string
  }))
  default     = []
  description = "Custom inbound rules to be added to the security group."
}

variable "vnet_rg_name" {
  type        = string
  default     = ""
  description = "Resource group name of vnet"
}

variable "private_dns_rg_name" {
  type        = string
  description = "The private DNS zone resource group name. Required when private_dns_zone_name is set."
  default     = ""
}

variable "private_dns_zone_name" {
  type        = string
  description = "The private DNS zone name."
  default     = ""
}

variable "private_dns_zone_use" {
  type        = bool
  description = "Determines whether to use private DNS zone. Required for LB dns name."
  default     = true
}

variable "sg_id" {
  type        = string
  description = "The security group id."
  default     = ""
}

variable "create_nat_gateway" {
  type        = bool
  default     = false
  description = "NAT needs to be created when no public ip is assigned to the backend, to allow internet access"
}
