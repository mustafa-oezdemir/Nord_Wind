package identity

import "time"

// Customer ist vor NW-11 nur der referenzierbare Eigentümer einer Bestellung.
type Customer struct {
	ID        string
	Email     string
	CreatedAt time.Time
}
