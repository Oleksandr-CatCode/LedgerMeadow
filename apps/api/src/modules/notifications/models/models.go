package models

import "time"

type Notification struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	EntityType *string    `json:"entity_type"`
	EntityID   *string    `json:"entity_id"`
	ReadAt     *time.Time `json:"read_at"`
	CreatedAt  time.Time  `json:"created_at"`
}
type Preference struct {
	Type         string `json:"type"`
	InAppEnabled bool   `json:"in_app_enabled"`
	EmailEnabled bool   `json:"email_enabled"`
}
