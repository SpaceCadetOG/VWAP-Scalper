package models

// CandleEvent is a normalized OHLCV candle.
type CandleEvent struct {
	Venue      Venue
	Symbol     string
	Interval   string
	OpenTimeMs int64
	CloseTimeMs int64
	Open       string
	High       string
	Low        string
	Close      string
	Volume     string
	TradeCount int64
}

// BBOEvent is normalized best bid/offer.
type BBOEvent struct {
	Venue    Venue
	Symbol   string
	TsMs     int64
	BidPx    string
	BidSz    string
	AskPx    string
	AskSz    string
}

// TradeEvent is a normalized tape event.
type TradeEvent struct {
	Venue  Venue
	Symbol string
	TsMs   int64
	Px     string
	Sz     string
	Side   Side
	ID     string
}

// BookLevel is one side level.
type BookLevel struct {
	Px string
	Sz string
}

// DepthEvent is a normalized depth snapshot/increment.
type DepthEvent struct {
	Venue  Venue
	Symbol string
	TsMs   int64
	Bids   []BookLevel
	Asks   []BookLevel
	Seq    int64
}
