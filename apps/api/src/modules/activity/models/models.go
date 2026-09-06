package models

import "time"

type Summary struct {
	InboxCount              int       `json:"inbox_count"`
	UnreadNotificationCount int       `json:"unread_notification_count"`
	UpdatedAt               time.Time `json:"updated_at"`
}
