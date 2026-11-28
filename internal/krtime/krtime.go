// Package krtime pins Korean market time. Broker APIs report wall-clock times
// and trading dates in KST with no offset, and the release image carries no
// tzdata, so time.Local cannot be relied on to be Asia/Seoul.
package krtime

import "time"

// KST is Korea Standard Time (UTC+9, no daylight saving).
var KST = time.FixedZone("KST", 9*60*60)

// Now returns the current time in KST.
func Now() time.Time {
	return time.Now().In(KST)
}

// Today returns the current KST date as YYYYMMDD, the form broker APIs take.
func Today() string {
	return Now().Format("20060102")
}
