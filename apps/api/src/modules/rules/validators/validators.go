package validators

import (
	"errors"
	"strings"

	"ledgermeadow/src/modules/rules/models"
)

var ErrInvalid = errors.New("invalid rule")

func Create(command models.Create) error {
	nameLength := len(strings.TrimSpace(command.Name))
	if nameLength < 1 || nameLength > 120 || len(command.Conditions) < 1 || len(command.Conditions) > 20 || len(command.Actions) < 1 || len(command.Actions) > 20 {
		return ErrInvalid
	}
	for _, condition := range command.Conditions {
		switch condition.Field {
		case "merchant", "amount", "category", "account", "transaction_type", "income_source", "space":
		default:
			return ErrInvalid
		}
		switch condition.Operator {
		case "contains", "is_exactly", "starts_with", "greater_than", "less_than":
		default:
			return ErrInvalid
		}
		valueLength := len(strings.TrimSpace(condition.Value))
		if valueLength < 1 || valueLength > 200 {
			return ErrInvalid
		}
	}
	for _, action := range command.Actions {
		switch action.Type {
		case "set_category", "allocate_space", "add_tag", "set_split", "mark_reviewed", "create_notification":
		default:
			return ErrInvalid
		}
		if len(action.Value) > 200 {
			return ErrInvalid
		}
	}
	return nil
}

func Update(command models.Update) error {
	if command.Name == nil && command.Enabled == nil && command.Conditions == nil && command.Actions == nil {
		return ErrInvalid
	}
	if command.Name != nil {
		length := len(strings.TrimSpace(*command.Name))
		if length < 1 || length > 120 {
			return ErrInvalid
		}
	}
	if command.Conditions != nil && validateConditions(*command.Conditions) != nil {
		return ErrInvalid
	}
	if command.Actions != nil && validateActions(*command.Actions) != nil {
		return ErrInvalid
	}
	return nil
}

func validateConditions(conditions []models.Condition) error {
	if len(conditions) < 1 || len(conditions) > 20 {
		return ErrInvalid
	}
	for _, condition := range conditions {
		switch condition.Field {
		case "merchant", "amount", "category", "account", "transaction_type", "income_source", "space":
		default:
			return ErrInvalid
		}
		switch condition.Operator {
		case "contains", "is_exactly", "starts_with", "greater_than", "less_than":
		default:
			return ErrInvalid
		}
		valueLength := len(strings.TrimSpace(condition.Value))
		if valueLength < 1 || valueLength > 200 {
			return ErrInvalid
		}
	}
	return nil
}

func validateActions(actions []models.Action) error {
	if len(actions) < 1 || len(actions) > 20 {
		return ErrInvalid
	}
	for _, action := range actions {
		switch action.Type {
		case "set_category", "allocate_space", "add_tag", "set_split", "mark_reviewed", "create_notification":
		default:
			return ErrInvalid
		}
		if len(action.Value) > 200 {
			return ErrInvalid
		}
	}
	return nil
}
