package models

import (
	"fmt"
	"math/big"
	"strings"
)

// Venue identifies an execution venue.
type Venue string

const (
	VenueHyperliquid Venue = "hyperliquid"
	VenueAster       Venue = "aster"
	VenueLighter     Venue = "lighter"
)

// SymbolSpec defines tradable market constraints for a venue symbol.
type SymbolSpec struct {
	Venue       Venue
	Symbol      string
	BaseAsset   string
	QuoteAsset  string
	TickSize    string
	LotSize     string
	MinNotional string
	MinQty      string
	MaxQty      string
}

// SymbolRegistry stores canonical-to-venue symbol mapping and constraints.
type SymbolRegistry struct {
	byCanonical map[string]map[Venue]SymbolSpec
}

func NewSymbolRegistry() *SymbolRegistry {
	return &SymbolRegistry{byCanonical: make(map[string]map[Venue]SymbolSpec)}
}

func (r *SymbolRegistry) Upsert(canonical string, spec SymbolSpec) {
	canon := strings.ToUpper(strings.TrimSpace(canonical))
	if _, ok := r.byCanonical[canon]; !ok {
		r.byCanonical[canon] = make(map[Venue]SymbolSpec)
	}
	r.byCanonical[canon][spec.Venue] = spec
}

func (r *SymbolRegistry) Get(canonical string, venue Venue) (SymbolSpec, bool) {
	canon := strings.ToUpper(strings.TrimSpace(canonical))
	byVenue, ok := r.byCanonical[canon]
	if !ok {
		return SymbolSpec{}, false
	}
	spec, ok := byVenue[venue]
	return spec, ok
}

// ValidateOrderInput checks qty/px against venue symbol constraints.
func ValidateOrderInput(spec SymbolSpec, qty string, px string) error {
	qtyR, err := parsePositiveDecimal("qty", qty)
	if err != nil {
		return err
	}
	pxR, err := parsePositiveDecimal("px", px)
	if err != nil {
		return err
	}

	tick, err := parsePositiveDecimal("tickSize", spec.TickSize)
	if err != nil {
		return err
	}
	lot, err := parsePositiveDecimal("lotSize", spec.LotSize)
	if err != nil {
		return err
	}

	if !isMultipleOf(qtyR, lot) {
		return fmt.Errorf("qty %s not aligned to lotSize %s", qty, spec.LotSize)
	}
	if !isMultipleOf(pxR, tick) {
		return fmt.Errorf("px %s not aligned to tickSize %s", px, spec.TickSize)
	}

	if spec.MinQty != "" {
		minQty, err := parsePositiveDecimal("minQty", spec.MinQty)
		if err != nil {
			return err
		}
		if qtyR.Cmp(minQty) < 0 {
			return fmt.Errorf("qty %s below minQty %s", qty, spec.MinQty)
		}
	}

	if spec.MaxQty != "" {
		maxQty, err := parsePositiveDecimal("maxQty", spec.MaxQty)
		if err != nil {
			return err
		}
		if qtyR.Cmp(maxQty) > 0 {
			return fmt.Errorf("qty %s above maxQty %s", qty, spec.MaxQty)
		}
	}

	if spec.MinNotional != "" {
		minNotional, err := parsePositiveDecimal("minNotional", spec.MinNotional)
		if err != nil {
			return err
		}
		notional := new(big.Rat).Mul(qtyR, pxR)
		if notional.Cmp(minNotional) < 0 {
			return fmt.Errorf("notional %s below minNotional %s", ratToString(notional), spec.MinNotional)
		}
	}

	return nil
}

func parsePositiveDecimal(field string, raw string) (*big.Rat, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	r := new(big.Rat)
	if _, ok := r.SetString(v); !ok {
		return nil, fmt.Errorf("invalid decimal for %s: %s", field, raw)
	}
	if r.Sign() <= 0 {
		return nil, fmt.Errorf("%s must be > 0", field)
	}
	return r, nil
}

func isMultipleOf(value *big.Rat, step *big.Rat) bool {
	q := new(big.Rat).Quo(value, step)
	return q.IsInt()
}

func ratToString(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	return r.FloatString(8)
}
