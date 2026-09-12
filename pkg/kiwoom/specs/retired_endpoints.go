// Package specs preserves compatibility for Kiwoom APIs retired on 2026-09-12 at 21:00 KST.
// Official notice: https://openapi.kiwoom.com/board/Board0101View?seqid=60
// These declarations preserve source compatibility; the retired APIs are not
// included in the active documented specs or request/response factories.
package specs

// Keep the previously generated public identifiers for source compatibility.
//revive:disable:var-naming

// KiwoomAPIIDKa10087 identifies a retired Kiwoom API.
//
// Deprecated: Kiwoom retired this API on 2026-09-12. No replacement was announced.
const KiwoomAPIIDKa10087 = "ka10087"

// KiwoomAPIIDKa10098 identifies a retired Kiwoom API.
//
// Deprecated: Kiwoom retired this API on 2026-09-12. No replacement was announced.
const KiwoomAPIIDKa10098 = "ka10098"

// KiwoomApiDostkMrkcondKa10087Request preserves the former API payload for source compatibility.
//
// Deprecated: Kiwoom retired this API on 2026-09-12.
type KiwoomApiDostkMrkcondKa10087Request struct {
	StkCd string `json:"stk_cd,omitempty"`
}

// KiwoomApiDostkRkinfoKa10098Request preserves the former API payload for source compatibility.
//
// Deprecated: Kiwoom retired this API on 2026-09-12.
type KiwoomApiDostkRkinfoKa10098Request struct {
	CrdCnd     string `json:"crd_cnd,omitempty"`
	MrktTp     string `json:"mrkt_tp,omitempty"`
	SortBase   string `json:"sort_base,omitempty"`
	StkCnd     string `json:"stk_cnd,omitempty"`
	TrdePrica  string `json:"trde_prica,omitempty"`
	TrdeQtyCnd string `json:"trde_qty_cnd,omitempty"`
}

// KiwoomApiDostkMrkcondKa10087Response preserves the former API payload for source compatibility.
//
// Deprecated: Kiwoom retired this API on 2026-09-12.
type KiwoomApiDostkMrkcondKa10087Response struct {
	BidReqBaseTm            string `json:"bid_req_base_tm,omitempty"`
	BuyBidTotReq            string `json:"buy_bid_tot_req,omitempty"`
	BuyBidTotReqJubPre      string `json:"buy_bid_tot_req_jub_pre,omitempty"`
	OvtBuyBidTotReq         string `json:"ovt_buy_bid_tot_req,omitempty"`
	OvtBuyBidTotReqJubPre   string `json:"ovt_buy_bid_tot_req_jub_pre,omitempty"`
	OvtSelBidTotReq         string `json:"ovt_sel_bid_tot_req,omitempty"`
	OvtSelBidTotReqJubPre   string `json:"ovt_sel_bid_tot_req_jub_pre,omitempty"`
	OvtSigpricAccTrdeQty    string `json:"ovt_sigpric_acc_trde_qty,omitempty"`
	OvtSigpricBuyBid1       string `json:"ovt_sigpric_buy_bid_1,omitempty"`
	OvtSigpricBuyBid2       string `json:"ovt_sigpric_buy_bid_2,omitempty"`
	OvtSigpricBuyBid3       string `json:"ovt_sigpric_buy_bid_3,omitempty"`
	OvtSigpricBuyBid4       string `json:"ovt_sigpric_buy_bid_4,omitempty"`
	OvtSigpricBuyBid5       string `json:"ovt_sigpric_buy_bid_5,omitempty"`
	OvtSigpricBuyBidJubPre1 string `json:"ovt_sigpric_buy_bid_jub_pre_1,omitempty"`
	OvtSigpricBuyBidJubPre2 string `json:"ovt_sigpric_buy_bid_jub_pre_2,omitempty"`
	OvtSigpricBuyBidJubPre3 string `json:"ovt_sigpric_buy_bid_jub_pre_3,omitempty"`
	OvtSigpricBuyBidJubPre4 string `json:"ovt_sigpric_buy_bid_jub_pre_4,omitempty"`
	OvtSigpricBuyBidJubPre5 string `json:"ovt_sigpric_buy_bid_jub_pre_5,omitempty"`
	OvtSigpricBuyBidQty1    string `json:"ovt_sigpric_buy_bid_qty_1,omitempty"`
	OvtSigpricBuyBidQty2    string `json:"ovt_sigpric_buy_bid_qty_2,omitempty"`
	OvtSigpricBuyBidQty3    string `json:"ovt_sigpric_buy_bid_qty_3,omitempty"`
	OvtSigpricBuyBidQty4    string `json:"ovt_sigpric_buy_bid_qty_4,omitempty"`
	OvtSigpricBuyBidQty5    string `json:"ovt_sigpric_buy_bid_qty_5,omitempty"`
	OvtSigpricBuyBidTotReq  string `json:"ovt_sigpric_buy_bid_tot_req,omitempty"`
	OvtSigpricCurPrc        string `json:"ovt_sigpric_cur_prc,omitempty"`
	OvtSigpricFluRt         string `json:"ovt_sigpric_flu_rt,omitempty"`
	OvtSigpricPredPre       string `json:"ovt_sigpric_pred_pre,omitempty"`
	OvtSigpricPredPreSig    string `json:"ovt_sigpric_pred_pre_sig,omitempty"`
	OvtSigpricSelBid1       string `json:"ovt_sigpric_sel_bid_1,omitempty"`
	OvtSigpricSelBid2       string `json:"ovt_sigpric_sel_bid_2,omitempty"`
	OvtSigpricSelBid3       string `json:"ovt_sigpric_sel_bid_3,omitempty"`
	OvtSigpricSelBid4       string `json:"ovt_sigpric_sel_bid_4,omitempty"`
	OvtSigpricSelBid5       string `json:"ovt_sigpric_sel_bid_5,omitempty"`
	OvtSigpricSelBidJubPre1 string `json:"ovt_sigpric_sel_bid_jub_pre_1,omitempty"`
	OvtSigpricSelBidJubPre2 string `json:"ovt_sigpric_sel_bid_jub_pre_2,omitempty"`
	OvtSigpricSelBidJubPre3 string `json:"ovt_sigpric_sel_bid_jub_pre_3,omitempty"`
	OvtSigpricSelBidJubPre4 string `json:"ovt_sigpric_sel_bid_jub_pre_4,omitempty"`
	OvtSigpricSelBidJubPre5 string `json:"ovt_sigpric_sel_bid_jub_pre_5,omitempty"`
	OvtSigpricSelBidQty1    string `json:"ovt_sigpric_sel_bid_qty_1,omitempty"`
	OvtSigpricSelBidQty2    string `json:"ovt_sigpric_sel_bid_qty_2,omitempty"`
	OvtSigpricSelBidQty3    string `json:"ovt_sigpric_sel_bid_qty_3,omitempty"`
	OvtSigpricSelBidQty4    string `json:"ovt_sigpric_sel_bid_qty_4,omitempty"`
	OvtSigpricSelBidQty5    string `json:"ovt_sigpric_sel_bid_qty_5,omitempty"`
	OvtSigpricSelBidTotReq  string `json:"ovt_sigpric_sel_bid_tot_req,omitempty"`
	SelBidTotReq            string `json:"sel_bid_tot_req,omitempty"`
	SelBidTotReqJubPre      string `json:"sel_bid_tot_req_jub_pre,omitempty"`
}

// KiwoomApiDostkRkinfoKa10098Response preserves the former API payload for source compatibility.
//
// Deprecated: Kiwoom retired this API on 2026-09-12.
type KiwoomApiDostkRkinfoKa10098Response struct {
	OvtSigpricFluRtRank []KiwoomApiDostkRkinfoKa10098ResponseItem `json:"ovt_sigpric_flu_rt_rank,omitempty"`
}

// KiwoomApiDostkRkinfoKa10098ResponseItem preserves the former API payload for source compatibility.
//
// Deprecated: Kiwoom retired this API on 2026-09-12.
type KiwoomApiDostkRkinfoKa10098ResponseItem struct {
	AccTrdePrica      string `json:"acc_trde_prica,omitempty"`
	AccTrdeQty        string `json:"acc_trde_qty,omitempty"`
	BuyTotReq         string `json:"buy_tot_req,omitempty"`
	CurPrc            string `json:"cur_prc,omitempty"`
	FluRt             string `json:"flu_rt,omitempty"`
	PredPre           string `json:"pred_pre,omitempty"`
	PredPreSig        string `json:"pred_pre_sig,omitempty"`
	Rank              string `json:"rank,omitempty"`
	SelTotReq         string `json:"sel_tot_req,omitempty"`
	StkCd             string `json:"stk_cd,omitempty"`
	StkNm             string `json:"stk_nm,omitempty"`
	TdyClosePric      string `json:"tdy_close_pric,omitempty"`
	TdyClosePricFluRt string `json:"tdy_close_pric_flu_rt,omitempty"`
}
