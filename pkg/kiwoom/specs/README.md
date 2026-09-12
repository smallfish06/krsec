# Kiwoom Documented Snapshot

`documented_endpoints.json` is a committed snapshot of Kiwoom documented REST specs.

- Runtime code does **not** call Kiwoom docs site.
- Generated files are built from this snapshot only.
- CI runs `make kiwoom-spec-check` to detect stale generated outputs.

## Refresh Flow

1. `make kiwoom-spec-refresh`
2. Review diff (`pkg/kiwoom/specs/documented_endpoints.json`, generated `.go` files)
3. Run live smoke/contract checks for critical endpoints
4. Commit

## Retired APIs

Kiwoom [announced](https://openapi.kiwoom.com/board/Board0101View?seqid=60)
the removal of `ka10087` (시간외단일가요청) and `ka10098`
(시간외단일가등락율순위요청), effective **2026-09-12 21:00 KST**.
The [official change table](https://bbn.kiwoom.com/bbs/VBbsNoticeImageView?apndnm=260910084701282YFH7.png)
explicitly lists both TR deletions. No replacement TR is specified.

These APIs are excluded from the current snapshot and active endpoint factories.
Their exported Go constants and payload types remain in `retired_endpoints.go`
with deprecation comments so existing callers still compile. This does not imply
that the upstream APIs remain available.
