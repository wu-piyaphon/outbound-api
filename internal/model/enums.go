package model

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusFilled   Status = "filled"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
)
