package specs_test

import (
	"testing"

	"github.com/smallfish06/krsec/pkg/kis/specs"
)

func TestRetiredFinanceAPIsPreserveTypesWithoutActiveRegistration(t *testing.T) {
	// Exercise the public names from an external package: source compatibility
	// must survive regeneration, while new documented calls must not use them.
	for _, tc := range []struct {
		path     string
		request  any
		response any
	}{
		{"/uapi/domestic-stock/v1/finance/balance-sheet",
			specs.KISDomesticStockV1FinanceBalanceSheetRequest{FidInputIscd: "005930"},
			specs.KISDomesticStockV1FinanceBalanceSheet{Output: []specs.KISDomesticStockV1FinanceBalanceSheetOutputItem{{StacYymm: "202512"}}}},
		{"/uapi/domestic-stock/v1/finance/financial-ratio",
			specs.KISDomesticStockV1FinanceFinancialRatioRequest{FidInputIscd: "005930"},
			specs.KISDomesticStockV1FinanceFinancialRatio{Output: []specs.KISDomesticStockV1FinanceFinancialRatioOutputItem{{StacYymm: "202512"}}}},
		{"/uapi/domestic-stock/v1/finance/growth-ratio",
			specs.KISDomesticStockV1FinanceGrowthRatioRequest{FidInputIscd: "005930"},
			specs.KISDomesticStockV1FinanceGrowthRatio{Output: []specs.KISDomesticStockV1FinanceGrowthRatioOutputItem{{StacYymm: "202512"}}}},
		{"/uapi/domestic-stock/v1/finance/income-statement",
			specs.KISDomesticStockV1FinanceIncomeStatementRequest{FidInputIscd: "005930"},
			specs.KISDomesticStockV1FinanceIncomeStatement{Output: []specs.KISDomesticStockV1FinanceIncomeStatementOutputItem{{StacYymm: "202512"}}}},
		{"/uapi/domestic-stock/v1/finance/other-major-ratios",
			specs.KISDomesticStockV1FinanceOtherMajorRatiosRequest{FidInputIscd: "005930"},
			specs.KISDomesticStockV1FinanceOtherMajorRatios{Output: []specs.KISDomesticStockV1FinanceOtherMajorRatiosOutputItem{{StacYymm: "202512"}}}},
		{"/uapi/domestic-stock/v1/finance/profit-ratio",
			specs.KISDomesticStockV1FinanceProfitRatioRequest{FidInputIscd: "005930"},
			specs.KISDomesticStockV1FinanceProfitRatio{Output: []specs.KISDomesticStockV1FinanceProfitRatioOutputItem{{StacYymm: "202512"}}}},
		{"/uapi/domestic-stock/v1/finance/stability-ratio",
			specs.KISDomesticStockV1FinanceStabilityRatioRequest{FidInputIscd: "005930"},
			specs.KISDomesticStockV1FinanceStabilityRatio{Output: []specs.KISDomesticStockV1FinanceStabilityRatioOutputItem{{StacYymm: "202512"}}}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if tc.request == nil || tc.response == nil {
				t.Fatal("missing legacy payload type")
			}
			if _, ok := specs.DocumentedKISEndpointSpecs[tc.path]; ok {
				t.Fatal("retired API is still active")
			}
			if specs.NewDocumentedEndpointRequest(tc.path) != nil || specs.NewDocumentedEndpointResponse(tc.path) != nil {
				t.Fatal("retired API still has an active factory")
			}
		})
	}
}
