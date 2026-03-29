package model

type Watchlist struct {
	Symbol   string `db:"symbol" json:"symbol"`
	IsActive bool   `db:"is_active" json:"is_active"`
}
