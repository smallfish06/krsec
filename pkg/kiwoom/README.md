# Kiwoom adapter

The raw adapter covers the committed documented REST endpoints. WebSocket
subscriptions and condition search are not implemented. The common quote and
OHLCV methods cover domestic markets; use raw REST endpoints for US data.

## Continuation

`CallEndpoint` retains its original response type. For paginated REST results,
use the optional `kiwoom.PageCaller` interface implemented by the built-in
adapter. `CallEndpointPage` returns one `kiwoom.EndpointPage`: `Data` has the same
type as the legacy response, while `ContYN` and `NextKey` carry the broker's
continuation headers. Send both fields from a response with `ContYN == "Y"` as
the `kiwoom.Continuation` argument to the next call. A blank continuation starts
a new query. Each call sends exactly one request, including order endpoints.

HTTP proxy routes preserve their existing JSON response and expose `cont-yn`
and `next-key` response headers. Send those headers back to request the next
page. The generic proxy also accepts a top-level body field:

```json
{"continuation": {"cont_yn": "Y", "next_key": "previous-response-key"}}
```

Common `GetPositions` calls collect all pages before returning. `GetOHLCV`
collects descending bars until the requested result limit or earliest date is
covered; older pages are then unnecessary. With no such bound it collects all
pages. Both reject intermediate failures, invalid or repeated cursors, and
responses exceeding 100 pages or 100,000 rows, returning an error instead of
partial data. Charts also reject malformed dates or bars out of descending
order. The returned bars respect the requested date range and result limit.

## Domestic market selection

Common market data calls map `market=NXT` to a `_NX` stock-code suffix and
`market=SOR` to `_AL`. Explicit stock-code suffixes must agree with a supplied
market. If the market is omitted, an explicit suffix selects that venue. Quote
responses contain the plain stock code and the selected market separately.

`GetPositions` uses the documented aggregate query mode (`qry_tp=1`).
`GetBalance` reads withdrawable cash from `pymn_alow_amt`; if omitted by Kiwoom,
`unavailable_fields` contains `withdrawable_cash` instead of assuming it equals
buying power.
