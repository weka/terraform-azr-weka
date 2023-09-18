data "azurerm_client_config" "current" {}

data "azurerm_resource_group" "rg" {
  name = var.rg_name
}

data "azurerm_subnet" "subnet" {
  resource_group_name  = var.vnet_rg_name
  virtual_network_name = var.vnet_name
  name                 = var.subnet_name
}

locals {
  disk_size               = var.disk_size + var.traces_per_frontend * var.frontend_cores_num
  private_nic_first_index = var.assign_public_ip ? 1 : 0

  init_script = templatefile("${path.module}/init.sh", {
    apt_repo_server  = var.apt_repo_server
    nics_num         = var.nics_numbers
    subnet_range     = data.azurerm_subnet.subnet.address_prefix
    disk_size        = local.disk_size
    install_weka_url = var.install_weka_url
    key_vault_url    = data.azurerm_key_vault.this.vault_uri
  })

  deploy_script = templatefile("${path.module}/deploy_protocol_gateways.sh", {
    frontend_cores_num = var.frontend_cores_num
    subnet_prefixes    = data.azurerm_subnet.subnet.address_prefix
    backend_lb_ip      = var.backend_lb_ip
    nics_num           = var.nics_numbers
    key_vault_url      = data.azurerm_key_vault.this.vault_uri
  })

  setup_nfs_protocol_script = templatefile("${path.module}/setup_nfs.sh", {
    gateways_name        = var.gateways_name
    interface_group_name = var.interface_group_name
    client_group_name    = var.client_group_name
  })

  setup_smb_protocol_script = templatefile("${path.module}/setup_smb.sh", {
    cluster_name        = var.smb_cluster_name
    domain_name         = var.smb_domain_name
    domain_netbios_name = var.smb_domain_netbios_name
    smbw_enabled        = var.smbw_enabled
    dns_ip              = var.smb_dns_ip_address
    gateways_number     = var.gateways_number
    gateways_name       = var.gateways_name
    frontend_cores_num  = var.frontend_cores_num
    share_name          = var.smb_share_name
  })

  protocol_script = var.protocol == "NFS" ? local.setup_nfs_protocol_script : local.setup_smb_protocol_script

  setup_protocol_script = var.setup_protocol ? local.protocol_script : ""

  custom_data_parts = [
    local.init_script, local.deploy_script, local.setup_protocol_script
  ]
  custom_data = join("\n", local.custom_data_parts)
}

resource "azurerm_linux_virtual_machine_scale_set" "vmss" {
  name                            = "${var.gateways_name}-vmss"
  location                        = data.azurerm_resource_group.rg.location
  resource_group_name             = var.rg_name
  instances                       = var.gateways_number
  sku                             = var.instance_type
  upgrade_mode                    = "Manual"
  computer_name_prefix            = "${var.gateways_name}-vmss"
  admin_username                  = var.vm_username
  custom_data                     = base64encode(local.custom_data)
  proximity_placement_group_id    = var.ppg_id
  disable_password_authentication = true
  source_image_id                 = var.source_image_id
  tags                            = merge(var.tags_map, { "weka_protocol_gateways" : var.gateways_name, "user_id" : data.azurerm_client_config.current.object_id })

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "StandardSSD_LRS"
  }

  data_disk {
    lun                  = 0
    caching              = "ReadWrite"
    create_option        = "Empty"
    disk_size_gb         = local.disk_size
    storage_account_type = "StandardSSD_LRS"
  }

  admin_ssh_key {
    username   = var.vm_username
    public_key = var.ssh_public_key
  }

  identity {
    type = "SystemAssigned"
  }

  dynamic "network_interface" {
    for_each = range(local.private_nic_first_index)
    content {
      name                          = "${var.gateways_name}-primary-nic-${network_interface.value}"
      network_security_group_id     = var.sg_id
      primary                       = true
      enable_accelerated_networking = true
      ip_configuration {
        primary   = true
        name      = "ipconfig0"
        subnet_id = data.azurerm_subnet.subnet.id
        public_ip_address {
          name = "${var.gateways_name}-public-ip"
        }
      }

      // secondary ips (floating ip)
      dynamic "ip_configuration" {
        for_each = range(var.secondary_ips_per_nic)
        content {
          name      = "secondary${ip_configuration.value}"
          subnet_id = data.azurerm_subnet.subnet.id
        }
      }
    }
  }
  dynamic "network_interface" {
    for_each = range(local.private_nic_first_index, 1)
    content {
      name                          = "${var.gateways_name}-primary-nic-${network_interface.value}"
      network_security_group_id     = var.sg_id
      primary                       = true
      enable_accelerated_networking = true
      ip_configuration {
        primary   = true
        name      = "ipconfig0"
        subnet_id = data.azurerm_subnet.subnet.id
      }
      // secondary ips (floating ip)
      dynamic "ip_configuration" {
        for_each = range(var.secondary_ips_per_nic)
        content {
          name      = "secondary${ip_configuration.value}"
          subnet_id = data.azurerm_subnet.subnet.id
        }
      }
    }
  }

  dynamic "network_interface" {
    for_each = range(1, var.nics_numbers)
    content {
      name                          = "${var.gateways_name}-secondary-nic-${network_interface.value}"
      network_security_group_id     = var.sg_id
      primary                       = false
      enable_accelerated_networking = true
      ip_configuration {
        primary   = true
        name      = "ipconfig-${network_interface.value}"
        subnet_id = data.azurerm_subnet.subnet.id
      }
    }
  }

  lifecycle {
    ignore_changes = [instances, custom_data, location, tags]
    precondition {
      condition     = var.protocol == "NFS" ? var.gateways_number >= 1 : var.gateways_number >= 3 && var.gateways_number <= 8
      error_message = "The amount of protocol gateways should be at least 1 for NFS and at least 3 and at most 8 for SMB."
    }
    precondition {
      condition     = var.protocol == "SMB" ? var.smb_domain_name != "" : true
      error_message = "The SMB domain name should be set when deploying SMB protocol gateways."
    }
    precondition {
      condition     = var.protocol == "SMB" ? var.secondary_ips_per_nic <= 3 : true
      error_message = "The number of secondary IPs per single NIC per protocol gateway virtual machine must be at most 3 for SMB."
    }
  }
}

data "azurerm_key_vault" "this" {
  name                = var.key_vault_name
  resource_group_name = var.rg_name
}

resource "azurerm_key_vault_access_policy" "gateways_vmss_key_vault" {
  key_vault_id = data.azurerm_key_vault.this.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = azurerm_linux_virtual_machine_scale_set.vmss.identity[0].principal_id
  secret_permissions = [
    "Get",
  ]
  depends_on = [azurerm_linux_virtual_machine_scale_set.vmss]
}

resource "azurerm_role_assignment" "gateways_vmss_key_vault" {
  scope                = data.azurerm_key_vault.this.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_linux_virtual_machine_scale_set.vmss.identity[0].principal_id
  depends_on           = [azurerm_linux_virtual_machine_scale_set.vmss]
}
