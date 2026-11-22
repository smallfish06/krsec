// Package specs preserves compatibility for KIS domestic-stock finance APIs
// that the KIS Developers API guide no longer lists (removed by 2026-11-22):
//
//	/uapi/domestic-stock/v1/finance/{balance-sheet,financial-ratio,growth-ratio,
//	income-statement,other-major-ratios,profit-ratio,stability-ratio}
//
// These declarations preserve source compatibility; the removed APIs are not
// included in the active documented specs or request/response factories.
package specs

// KISDomesticStockV1FinanceBalanceSheet preserves the former /uapi/domestic-stock/v1/finance/balance-sheet response for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceBalanceSheet struct {
	DocumentedResponseBase
	Output DocumentedSlice[KISDomesticStockV1FinanceBalanceSheetOutputItem] `json:"output,omitempty"`
}

// KISDomesticStockV1FinanceBalanceSheetOutputItem preserves the former /uapi/domestic-stock/v1/finance/balance-sheet response item for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceBalanceSheetOutputItem struct {
	StacYymm  string `json:"stac_yymm"`
	Cras      string `json:"cras"`
	Fxas      string `json:"fxas"`
	TotalAset string `json:"total_aset"`
	FlowLblt  string `json:"flow_lblt"`
	FixLblt   string `json:"fix_lblt"`
	TotalLblt string `json:"total_lblt"`
	Cpfn      string `json:"cpfn"`
	CfpSurp   string `json:"cfp_surp"`
	PrfiSurp  string `json:"prfi_surp"`
	TotalCptl string `json:"total_cptl"`
}

// KISDomesticStockV1FinanceBalanceSheetRequest preserves the former /uapi/domestic-stock/v1/finance/balance-sheet request payload for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceBalanceSheetRequest struct {
	FidDivClsCode      string `json:"FID_DIV_CLS_CODE"`
	FidCondMrktDivCode string `json:"fid_cond_mrkt_div_code"`
	FidInputIscd       string `json:"fid_input_iscd"`
}

// KISDomesticStockV1FinanceFinancialRatio preserves the former /uapi/domestic-stock/v1/finance/financial-ratio response for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceFinancialRatio struct {
	DocumentedResponseBase
	Output DocumentedSlice[KISDomesticStockV1FinanceFinancialRatioOutputItem] `json:"output,omitempty"`
}

// KISDomesticStockV1FinanceFinancialRatioOutputItem preserves the former /uapi/domestic-stock/v1/finance/financial-ratio response item for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceFinancialRatioOutputItem struct {
	StacYymm     string `json:"stac_yymm"`
	Grs          string `json:"grs"`
	BsopPrfiInrt string `json:"bsop_prfi_inrt"`
	NtinInrt     string `json:"ntin_inrt"`
	RoeVal       string `json:"roe_val"`
	Eps          string `json:"eps"`
	Sps          string `json:"sps"`
	Bps          string `json:"bps"`
	RsrvRate     string `json:"rsrv_rate"`
	LbltRate     string `json:"lblt_rate"`
}

// KISDomesticStockV1FinanceFinancialRatioRequest preserves the former /uapi/domestic-stock/v1/finance/financial-ratio request payload for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceFinancialRatioRequest struct {
	FidDivClsCode      string `json:"FID_DIV_CLS_CODE"`
	FidCondMrktDivCode string `json:"fid_cond_mrkt_div_code"`
	FidInputIscd       string `json:"fid_input_iscd"`
}

// KISDomesticStockV1FinanceGrowthRatio preserves the former /uapi/domestic-stock/v1/finance/growth-ratio response for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceGrowthRatio struct {
	DocumentedResponseBase
	Output DocumentedSlice[KISDomesticStockV1FinanceGrowthRatioOutputItem] `json:"output,omitempty"`
}

// KISDomesticStockV1FinanceGrowthRatioOutputItem preserves the former /uapi/domestic-stock/v1/finance/growth-ratio response item for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceGrowthRatioOutputItem struct {
	StacYymm     string `json:"stac_yymm"`
	Grs          string `json:"grs"`
	BsopPrfiInrt string `json:"bsop_prfi_inrt"`
	EqutInrt     string `json:"equt_inrt"`
	TotlAsetInrt string `json:"totl_aset_inrt"`
}

// KISDomesticStockV1FinanceGrowthRatioRequest preserves the former /uapi/domestic-stock/v1/finance/growth-ratio request payload for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceGrowthRatioRequest struct {
	FidCondMrktDivCode string `json:"fid_cond_mrkt_div_code"`
	FidDivClsCode      string `json:"fid_div_cls_code"`
	FidInputIscd       string `json:"fid_input_iscd"`
}

// KISDomesticStockV1FinanceIncomeStatement preserves the former /uapi/domestic-stock/v1/finance/income-statement response for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceIncomeStatement struct {
	DocumentedResponseBase
	Output DocumentedSlice[KISDomesticStockV1FinanceIncomeStatementOutputItem] `json:"output,omitempty"`
}

// KISDomesticStockV1FinanceIncomeStatementOutputItem preserves the former /uapi/domestic-stock/v1/finance/income-statement response item for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceIncomeStatementOutputItem struct {
	StacYymm     string `json:"stac_yymm"`
	SaleAccount  string `json:"sale_account"`
	SaleCost     string `json:"sale_cost"`
	SaleTotlPrfi string `json:"sale_totl_prfi"`
	DeprCost     string `json:"depr_cost"`
	SellMang     string `json:"sell_mang"`
	BsopPrti     string `json:"bsop_prti"`
	BsopNonErnn  string `json:"bsop_non_ernn"`
	BsopNonExpn  string `json:"bsop_non_expn"`
	OpPrfi       string `json:"op_prfi"`
	SpecPrfi     string `json:"spec_prfi"`
	SpecLoss     string `json:"spec_loss"`
	ThtrNtin     string `json:"thtr_ntin"`
}

// KISDomesticStockV1FinanceIncomeStatementRequest preserves the former /uapi/domestic-stock/v1/finance/income-statement request payload for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceIncomeStatementRequest struct {
	FidDivClsCode      string `json:"FID_DIV_CLS_CODE"`
	FidCondMrktDivCode string `json:"fid_cond_mrkt_div_code"`
	FidInputIscd       string `json:"fid_input_iscd"`
}

// KISDomesticStockV1FinanceOtherMajorRatios preserves the former /uapi/domestic-stock/v1/finance/other-major-ratios response for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceOtherMajorRatios struct {
	DocumentedResponseBase
	Output DocumentedSlice[KISDomesticStockV1FinanceOtherMajorRatiosOutputItem] `json:"output,omitempty"`
}

// KISDomesticStockV1FinanceOtherMajorRatiosOutputItem preserves the former /uapi/domestic-stock/v1/finance/other-major-ratios response item for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceOtherMajorRatiosOutputItem struct {
	StacYymm   string `json:"stac_yymm"`
	PayoutRate string `json:"payout_rate"`
	Eva        string `json:"eva"`
	Ebitda     string `json:"ebitda"`
	EvEbitda   string `json:"ev_ebitda"`
}

// KISDomesticStockV1FinanceOtherMajorRatiosRequest preserves the former /uapi/domestic-stock/v1/finance/other-major-ratios request payload for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceOtherMajorRatiosRequest struct {
	FidCondMrktDivCode string `json:"fid_cond_mrkt_div_code"`
	FidDivClsCode      string `json:"fid_div_cls_code"`
	FidInputIscd       string `json:"fid_input_iscd"`
}

// KISDomesticStockV1FinanceProfitRatio preserves the former /uapi/domestic-stock/v1/finance/profit-ratio response for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceProfitRatio struct {
	DocumentedResponseBase
	Output DocumentedSlice[KISDomesticStockV1FinanceProfitRatioOutputItem] `json:"output,omitempty"`
}

// KISDomesticStockV1FinanceProfitRatioOutputItem preserves the former /uapi/domestic-stock/v1/finance/profit-ratio response item for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceProfitRatioOutputItem struct {
	StacYymm         string `json:"stac_yymm"`
	CptlNtinRate     string `json:"cptl_ntin_rate"`
	SelfCptlNtinInrt string `json:"self_cptl_ntin_inrt"`
	SaleNtinRate     string `json:"sale_ntin_rate"`
	SaleTotlRate     string `json:"sale_totl_rate"`
}

// KISDomesticStockV1FinanceProfitRatioRequest preserves the former /uapi/domestic-stock/v1/finance/profit-ratio request payload for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceProfitRatioRequest struct {
	FidDivClsCode      string `json:"FID_DIV_CLS_CODE"`
	FidCondMrktDivCode string `json:"fid_cond_mrkt_div_code"`
	FidInputIscd       string `json:"fid_input_iscd"`
}

// KISDomesticStockV1FinanceStabilityRatio preserves the former /uapi/domestic-stock/v1/finance/stability-ratio response for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceStabilityRatio struct {
	DocumentedResponseBase
	Output DocumentedSlice[KISDomesticStockV1FinanceStabilityRatioOutputItem] `json:"output,omitempty"`
}

// KISDomesticStockV1FinanceStabilityRatioOutputItem preserves the former /uapi/domestic-stock/v1/finance/stability-ratio response item for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceStabilityRatioOutputItem struct {
	StacYymm string `json:"stac_yymm"`
	LbltRate string `json:"lblt_rate"`
	BramDepn string `json:"bram_depn"`
	CrntRate string `json:"crnt_rate"`
	QuckRate string `json:"quck_rate"`
}

// KISDomesticStockV1FinanceStabilityRatioRequest preserves the former /uapi/domestic-stock/v1/finance/stability-ratio request payload for source compatibility.
//
// Deprecated: the KIS Developers API guide no longer documents this API.
type KISDomesticStockV1FinanceStabilityRatioRequest struct {
	FidCondMrktDivCode string `json:"fid_cond_mrkt_div_code"`
	FidDivClsCode      string `json:"fid_div_cls_code"`
	FidInputIscd       string `json:"fid_input_iscd"`
}
