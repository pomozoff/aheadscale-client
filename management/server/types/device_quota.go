package types

// RegistrationDeviceQuotaPolicyID identifies the persisted policy for new accounts.
const RegistrationDeviceQuotaPolicyID = "registration"

// AccountDeviceQuota is operator-managed and intentionally separate from API settings.
// A nil maximum is unlimited; zero prevents additional registrations.
type AccountDeviceQuota struct {
	AccountID  string `gorm:"primaryKey;not null"`
	MaxDevices *int64 `gorm:"check:account_device_quota_nonnegative,max_devices IS NULL OR max_devices >= 0"`
}

func (AccountDeviceQuota) TableName() string { return "account_device_quotas" }

// DeviceQuotaPolicy supplies the initial quota copied when an account is created.
type DeviceQuotaPolicy struct {
	ID         string `gorm:"primaryKey;not null"`
	MaxDevices *int64 `gorm:"check:device_quota_policy_nonnegative,max_devices IS NULL OR max_devices >= 0"`
}

func (DeviceQuotaPolicy) TableName() string { return "device_quota_policies" }
