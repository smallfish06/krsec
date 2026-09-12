package broker

import (
	"encoding/json"
	"testing"
)

func TestBalanceJSONDistinguishesUnavailableFromZero(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		balance := Balance{AccountID: "account", Cash: 0, TotalAssets: 0}
		if unavailable {
			balance.UnavailableFields = []string{"cash", "total_assets", "withdrawable_cash"}
		}
		data, err := json.Marshal(balance)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		_, cashPresent := fields["cash"]
		_, totalPresent := fields["total_assets"]
		if cashPresent == unavailable || totalPresent == unavailable {
			t.Fatalf("unavailable=%v, JSON=%s", unavailable, data)
		}
		if fields["account_id"] != "account" {
			t.Fatalf("identity lost: %s", data)
		}
	}
}
