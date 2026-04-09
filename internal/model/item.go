package model

import "time"

type Item struct {
	ID               string
	Identifier       string
	Title            string
	Description      string
	Priority         int
	State            string
	BranchName       string
	URL              string
	AssigneeID       string
	Labels           []string
	BlockedBy        []string
	AssignedToWorker bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
