data "azurerm_client_config" "current" {}

resource "azurerm_key_vault" "key_vault" {
  name                        = "${local.alphanumeric_prefix_name}-${local.alphanumeric_cluster_name}-key-vault"
  location                    = data.azurerm_resource_group.rg.location
  resource_group_name         = var.rg_name
  enabled_for_deployment      = true
  tenant_id                   = data.azurerm_client_config.current.tenant_id
  purge_protection_enabled    = false
  sku_name = "standard"
  tags = merge(var.tags_map, {"weka_cluster": var.cluster_name})
  lifecycle {
    ignore_changes = [tags]
  }
}

resource "azurerm_key_vault_access_policy" "function-app-get-secret-permission" {
  key_vault_id = azurerm_key_vault.key_vault.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = azurerm_linux_function_app.function_app.identity[0].principal_id

  secret_permissions = [
    "Get",
  ]

  depends_on = [azurerm_key_vault.key_vault,azurerm_linux_function_app.function_app]
}

resource "azurerm_key_vault_access_policy" "key_vault_access_policy" {
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = data.azurerm_client_config.current.object_id
  key_vault_id = azurerm_key_vault.key_vault.id
  key_permissions = [
    "Get","List","Update","Create","Import","Delete","Recover","Backup","Purge",
  ]
  secret_permissions = [
    "Get","List","Delete","Recover","Backup","Restore","Set","Purge",
  ]
  storage_permissions = [
    "Get",
    "List",
  ]

  depends_on = [azurerm_key_vault.key_vault]
}

resource "azurerm_key_vault_secret" "public-ssh-keys" {
  count        = var.ssh_public_key == null ? 1 : 0
  name         = "public-key"
  value        = tls_private_key.ssh_key[0].public_key_openssh
  key_vault_id = azurerm_key_vault.key_vault.id
  tags         = merge(var.tags_map, {"weka_cluster": var.cluster_name})
  lifecycle {
    ignore_changes  = [value,tags]
  }
  depends_on   = [azurerm_key_vault.key_vault, tls_private_key.ssh_key, azurerm_key_vault_access_policy.key_vault_access_policy]
}

resource "azurerm_key_vault_secret" "private-ssh-keys" {
  count        = var.ssh_public_key == null ? 1 : 0
  name         = "private-key"
  value        = tls_private_key.ssh_key[0].private_key_pem
  key_vault_id = azurerm_key_vault.key_vault.id
  tags         = merge(var.tags_map, {"weka_cluster": var.cluster_name})
  lifecycle {
    ignore_changes  = [value,tags]
  }
  depends_on   = [azurerm_key_vault.key_vault, tls_private_key.ssh_key, azurerm_key_vault_access_policy.key_vault_access_policy]
}

data "azurerm_function_app_host_keys" "function_keys" {
  name                = azurerm_linux_function_app.function_app.name
  resource_group_name = data.azurerm_resource_group.rg.name
  depends_on          = [azurerm_linux_function_app.function_app]
}

resource "azurerm_key_vault_secret" "function_app_default_key" {
  name         = "function-app-default-key"
  value        = data.azurerm_function_app_host_keys.function_keys.default_function_key
  key_vault_id = azurerm_key_vault.key_vault.id
  tags         = merge(var.tags_map, {"weka_cluster": var.cluster_name})
  depends_on   = [azurerm_key_vault.key_vault, azurerm_key_vault_access_policy.key_vault_access_policy]
  lifecycle {
    ignore_changes = [tags]
  }
}

resource "azurerm_key_vault_secret" "get_weka_io_token" {
  name         = "get-weka-io-token"
  value        = var.get_weka_io_token
  key_vault_id = azurerm_key_vault.key_vault.id
  tags         = merge(var.tags_map, {"weka_cluster": var.cluster_name})
  depends_on   = [azurerm_key_vault.key_vault, azurerm_key_vault_access_policy.key_vault_access_policy]
  lifecycle {
    ignore_changes = [tags]
  }
}

resource "random_password" "weka_password" {
  length  = 16
  lower   = true
  min_lower = 1
  upper   = true
  min_upper = 1
  numeric = true
  min_numeric = 1
  special = false
}

resource "azurerm_key_vault_secret" "weka_password_secret" {
  name         = "weka-password"
  value        = random_password.weka_password.result
  key_vault_id = azurerm_key_vault.key_vault.id
  tags         = merge(var.tags_map, {"weka_cluster": var.cluster_name})
  lifecycle {
    ignore_changes = [value,tags]
  }
  depends_on   = [azurerm_key_vault.key_vault, random_password.weka_password,azurerm_key_vault_access_policy.key_vault_access_policy]
}
