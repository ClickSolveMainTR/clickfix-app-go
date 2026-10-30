package rbac

// Integration management uses existing tenant permission groups; per-operation RBAC
// is checked again by the bridged resource route. No new frontend permission surface.
const (
	PermTenantAutomationRead  = PermTenantAutomationsRead
	PermTenantAutomationWrite = PermTenantAutomationsWrite
	PermTenantWebhooksRead    = PermTenantChannelsRead
	PermTenantWebhooksWrite   = PermTenantChannelsManage
	PermTenantAPIKeysManage   = PermTenantSettingsWrite
	PermTenantMCPConnect      = PermTenantSettingsRead
	PermTenantMCPWrite        = PermTenantSettingsWrite
)
