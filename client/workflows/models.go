// Copyright 2021 Northern.tech AS
//
//    All Rights Reserved

package workflows

const (
	ServiceInventory = "inventory"
)

type ReindexWorkflow struct {
	RequestID string `json:"request_id"`
	TenantID  string `json:"tenant_id"`
	DeviceID  string `json:"device_id"`
	Service   string `json:"service"`
}
