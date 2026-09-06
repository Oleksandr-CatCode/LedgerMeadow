package validators

import (
	"errors"
	"strings"

	transactions "ledgermeadow/src/modules/transactions/types"
	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
)

var ErrInvalidUpdate = errors.New("invalid transaction update")
var ErrInvalidFilters = errors.New("invalid transaction filters")
var ErrInvalidBulkUpdate = errors.New("invalid bulk transaction update")

func Update(update transactions.Update) error {
	if update.CategoryID == nil && update.SpaceID == nil && update.ReviewStatus == nil && update.Visibility == nil {
		return ErrInvalidUpdate
	}
	if update.CategoryID != nil && *update.CategoryID != "" && !validate.UUID(*update.CategoryID) {
		return ErrInvalidUpdate
	}
	if update.SpaceID != nil && *update.SpaceID != "" && !validate.UUID(*update.SpaceID) {
		return ErrInvalidUpdate
	}
	if update.ReviewStatus != nil {
		switch *update.ReviewStatus {
		case "NEEDS_REVIEW", "REVIEWED", "IGNORED":
		default:
			return ErrInvalidUpdate
		}
	}
	if update.Visibility != nil && *update.Visibility != "PRIVATE" && *update.Visibility != "HOUSEHOLD" {
		return ErrInvalidUpdate
	}
	return nil
}

func Filters(filters transactions.ListFilters) error {
	for _, id := range []*string{filters.AccountID, filters.CategoryID, filters.SpaceID} {
		if id != nil && !validate.UUID(*id) {
			return ErrInvalidFilters
		}
	}
	if filters.ReviewStatus != nil {
		switch *filters.ReviewStatus {
		case "NEEDS_REVIEW", "REVIEWED", "IGNORED":
		default:
			return ErrInvalidFilters
		}
	}
	if filters.Query != nil {
		length := len(strings.TrimSpace(*filters.Query))
		if length < 2 || length > 80 {
			return ErrInvalidFilters
		}
	}
	return nil
}

func Bulk(command transactions.BulkUpdate) error {
	if len(command.TransactionIDs) < 1 || len(command.TransactionIDs) > 50 || command.Update.Visibility != nil {
		return ErrInvalidBulkUpdate
	}
	if err := Update(command.Update); err != nil {
		return ErrInvalidBulkUpdate
	}
	seen := make(map[shared.TransactionID]struct{}, len(command.TransactionIDs))
	for _, id := range command.TransactionIDs {
		if !validate.UUID(string(id)) {
			return ErrInvalidBulkUpdate
		}
		if _, exists := seen[id]; exists {
			return ErrInvalidBulkUpdate
		}
		seen[id] = struct{}{}
	}
	return nil
}

func ID(value string) error {
	if !validate.UUID(value) {
		return ErrInvalidUpdate
	}
	return nil
}
