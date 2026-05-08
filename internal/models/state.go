package models

// StateSignal is a normalized market-state engine output.
type StateSignal struct {
	State           MarketState
	ConfidenceScore int
	Invalidators    []string
	ExpiryMs        int64
}

// NormalizedError is a canonical adapter/risk/router error shape.
type NormalizedError struct {
	Code       ErrorCode
	Venue      Venue
	Message    string
	Retryable  bool
	RawCode    string
}
