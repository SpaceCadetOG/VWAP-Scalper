package models

// TTLConfig carries entry and hold timing limits for scalping paths.
type TTLConfig struct {
	SignalTsMs          int64
	MaxEntryDelayMs     int64
	NoFollowThroughMs   int64
	MaxHoldMs           int64
}

// OrderRequest is the canonical order intent consumed by adapters.
type OrderRequest struct {
	Venue         Venue
	AccountID     string
	Symbol        string
	Side          Side
	Type          OrderType
	TIF           TimeInForce
	Qty           string
	Px            string
	StopPx        string
	ReduceOnly    bool
	PostOnly      bool
	ClientOrderID string
	Tags          map[string]string
	ExpireAtMs    int64
	TTL           TTLConfig
}

// OrderAck captures submission acknowledgement status.
type OrderAck struct {
	Venue         Venue
	VenueOrderID  string
	ClientOrderID string
	Accepted      bool
	Reason        string
	TsMs          int64
}

// OrderUpdateEvent is canonical lifecycle update.
type OrderUpdateEvent struct {
	Venue         Venue
	VenueOrderID  string
	ClientOrderID string
	Symbol        string
	Status        OrderStatus
	FilledQty     string
	AvgFillPx     string
	Reason        string
	TsMs          int64
}

// FillEvent is canonical execution fill.
type FillEvent struct {
	Venue        Venue
	VenueOrderID string
	ClientOrderID string
	Symbol       string
	Side         Side
	Px           string
	Qty          string
	Fee          string
	FeeAsset     string
	LiquidityTag string
	TsMs         int64
}
