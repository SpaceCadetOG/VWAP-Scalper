package models

// Position is canonical perp position state.
type Position struct {
	Venue        Venue
	AccountID    string
	Symbol       string
	Qty          string
	EntryPx      string
	MarkPx       string
	UnrealizedPnL string
	RealizedPnL  string
	Leverage     string
	MarginMode   string
	UpdatedAtMs  int64
}

// Balance is canonical account balance state.
type Balance struct {
	Venue       Venue
	AccountID   string
	Asset       string
	Total       string
	Available   string
	Locked      string
	UpdatedAtMs int64
}

// FundingEvent is canonical funding payment/charge event.
type FundingEvent struct {
	Venue    Venue
	AccountID string
	Symbol   string
	Rate     string
	Amount   string
	TsMs     int64
}
