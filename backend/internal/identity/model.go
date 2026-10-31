package identity

import "time"

type Role string

const (
	CustomerRole Role = "customer"
	AdminRole    Role = "admin"
	EmployeeRole Role = "employee"
)

type Customer struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"-"`
}

type Session struct {
	Customer  Customer
	CSRFToken string
	ExpiresAt time.Time
}
