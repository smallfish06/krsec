package broker

import "encoding/json"

// MarshalJSON omits unavailable metrics instead of publishing placeholder zeros.
// Go callers must also inspect UnavailableFields before using a numeric field.
func (b Balance) MarshalJSON() ([]byte, error) {
	type plainBalance Balance
	data, err := json.Marshal(plainBalance(b))
	if err != nil || len(b.UnavailableFields) == 0 {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, name := range b.UnavailableFields {
		switch name {
		case "cash", "total_assets", "buying_power", "withdrawable_cash", "receivable_amount",
			"profit_loss", "profit_loss_pct", "position_cost", "position_value", "settlement_t1",
			"unsettled", "loan_balance", "cash_by_currency", "total_assets_by_currency",
			"buying_power_by_currency", "profit_loss_by_currency", "position_cost_by_currency", "position_value_by_currency":
			delete(fields, name)
		}
	}
	return json.Marshal(fields)
}
