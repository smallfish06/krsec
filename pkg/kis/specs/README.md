# KIS Documented Snapshot

`documented_endpoints.json` is a committed snapshot of KIS documented REST specs.

- Runtime code does **not** call KIS docs site.
- Generated files are built from this snapshot only.
- Request properties include both required and optional fields. Optional typed
  inputs use `omitempty`; runtime validation still requires only documented
  required fields. Raw map requests also accept optional inputs.
- CI runs `make kis-spec-check` to detect stale generated outputs.

## Refresh Flow

1. `make kis-spec-refresh`
2. Review diff (`pkg/kis/specs/documented_endpoints.json`, generated `.go` files)
3. Run live smoke/contract checks for critical endpoints
4. Commit

## Removed APIs

As of 2026-11-22 the [KIS Developers API guide](https://apiportal.koreainvestment.com/apiservice-apiservice)
no longer lists the domestic-stock finance APIs under
`/uapi/domestic-stock/v1/finance/`: `balance-sheet`, `financial-ratio`,
`growth-ratio`, `income-statement`, `other-major-ratios`, `profit-ratio` and
`stability-ratio`. No retirement notice was found, and the
[official sample repository](https://github.com/koreainvestment/open-trading-api)
still carries examples for them.

These APIs are excluded from the current snapshot and active endpoint factories.
Their exported payload types remain in `retired_endpoints.go` with deprecation
comments so existing callers still compile. This does not imply that the
upstream APIs remain available.
