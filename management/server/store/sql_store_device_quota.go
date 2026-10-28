package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/netbirdio/netbird/management/server/types"
	"github.com/netbirdio/netbird/shared/management/status"
)

// GetAccountDeviceQuota reads the current operator quota in the caller's transaction.
// A missing row is not an unlimited quota.
func (s *SqlStore) GetAccountDeviceQuota(ctx context.Context, lockStrength LockingStrength, accountID string) (*types.AccountDeviceQuota, error) {
	var quota types.AccountDeviceQuota
	result := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: string(lockStrength)}).Where(accountIDCondition, accountID).Take(&quota)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(status.NotFound, "device quota is not configured for account %s", accountID)
		}
		return nil, status.Errorf(status.Internal, "failed to read account device quota: %v", result.Error)
	}
	return &quota, nil
}

// initializeAccountDeviceQuota never replaces an existing per-account override.
// An absent policy leaves standard deployments unchanged; Aheadscale admission
// rejects missing quotas until the operator has configured the database policy.
func initializeAccountDeviceQuota(ctx context.Context, tx *gorm.DB, accountID string) error {
	var policy types.DeviceQuotaPolicy
	result := tx.WithContext(ctx).Where("id = ?", types.RegistrationDeviceQuotaPolicyID).Limit(1).Find(&policy)
	if result.Error != nil {
		return fmt.Errorf("read registration device quota policy: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil
	}
	result = tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&types.AccountDeviceQuota{AccountID: accountID, MaxDevices: policy.MaxDevices})
	if result.Error != nil {
		return fmt.Errorf("initialize account device quota: %w", result.Error)
	}
	return nil
}
