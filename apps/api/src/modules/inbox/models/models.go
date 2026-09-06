package models

import (
	"encoding/json"
	"time"
)

type Item struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Priority   string          `json:"priority"`
	EntityType string          `json:"entity_type"`
	EntityID   *string         `json:"entity_id"`
	Payload    json.RawMessage `json:"payload"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
}
type Page struct {
	Items      []Item
	NextCursor string
}
