package models

type Category struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	ParentID *string `json:"parent_id"`
}

type Create struct {
	Name     string
	Type     string
	ParentID *string
}
