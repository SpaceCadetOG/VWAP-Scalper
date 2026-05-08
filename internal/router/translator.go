package router

import (
	"fmt"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
)

type OrderRequest struct {
	Venue       models.Venue
	Symbol      string
	Side        Side
	NotionalUSD float64
	ReduceOnly  bool
}

func TranslateAllocation(intent Intent, venueSymbol string, alloc Allocation) (OrderRequest, error) {
	if alloc.NotionalUSD <= 0 {
		return OrderRequest{}, fmt.Errorf("allocation notional must be > 0")
	}
	if venueSymbol == "" {
		return OrderRequest{}, fmt.Errorf("venue symbol is required")
	}
	return OrderRequest{
		Venue:       alloc.Venue,
		Symbol:      venueSymbol,
		Side:        intent.Side,
		NotionalUSD: alloc.NotionalUSD,
		ReduceOnly:  intent.ReduceOnly,
	}, nil
}
