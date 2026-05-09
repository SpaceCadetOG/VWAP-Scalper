package replay

import (
	"fmt"
	"time"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/router"
)

type FillModel struct {
	SlippageBps float64
	FeeBps      float64
	LatencyMs   int64
}

type Engine struct {
	model FillModel
}

func NewEngine(model FillModel) *Engine {
	if model.LatencyMs <= 0 {
		model.LatencyMs = 250
	}
	return &Engine{model: model}
}

type VenueExecution struct {
	Venue         string
	NotionalUSD   float64
	SlippageCost  float64
	FeeCost       float64
	NetCost       float64
	OrderState    router.ExecState
	LatencyMs     int64
	SimulatedTime time.Time
}

type Result struct {
	SignalID      string
	Accepted      bool
	RejectReason  string
	Executions    []VenueExecution
	TotalNotional float64
	TotalNetCost  float64
}

func (e *Engine) ExecutePlan(plan router.Plan) Result {
	res := Result{
		SignalID: plan.Intent.SignalID,
		Accepted: plan.Accepted,
	}
	if !plan.Accepted {
		res.RejectReason = fmt.Sprintf("%s: %s", plan.Reason, plan.ReasonText)
		return res
	}

	now := time.Now().UTC()
	for _, a := range plan.Allocations {
		slippageCost := a.NotionalUSD * (e.model.SlippageBps / 10000.0)
		feeCost := a.NotionalUSD * (e.model.FeeBps / 10000.0)
		net := slippageCost + feeCost

		ve := VenueExecution{
			Venue:         string(a.Venue),
			NotionalUSD:   a.NotionalUSD,
			SlippageCost:  round4(slippageCost),
			FeeCost:       round4(feeCost),
			NetCost:       round4(net),
			OrderState:    router.ExecStateFilled,
			LatencyMs:     e.model.LatencyMs,
			SimulatedTime: now.Add(time.Duration(e.model.LatencyMs) * time.Millisecond),
		}
		res.Executions = append(res.Executions, ve)
		res.TotalNotional += a.NotionalUSD
		res.TotalNetCost += ve.NetCost
	}
	res.TotalNotional = round4(res.TotalNotional)
	res.TotalNetCost = round4(res.TotalNetCost)
	return res
}

func round4(v float64) float64 {
	const m = 10000.0
	if v >= 0 {
		return float64(int(v*m+0.5)) / m
	}
	return float64(int(v*m-0.5)) / m
}
