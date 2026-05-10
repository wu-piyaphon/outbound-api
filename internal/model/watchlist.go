package model

// Watchlist is a tradable symbol the bot subscribes to bar updates for.
// IsActive is the operator-facing flag toggled to add/remove a symbol without
// deleting the row.
type Watchlist struct {
	Symbol   string `db:"symbol" json:"symbol"`
	IsActive bool   `db:"is_active" json:"is_active"`
}
