// Package koreasecurities provides a unified gateway for Korean securities
// broker APIs.
//
// This module abstracts away the differences between Korean broker REST APIs
// (KIS, Kiwoom, LS, Toss, etc.) behind a single [github.com/smallfish06/krsec/pkg/broker.Broker] interface.
//
// Most users will import one of the sub-packages:
//
//   - [github.com/smallfish06/krsec/pkg/broker] — common interface and types
//   - [github.com/smallfish06/krsec/pkg/server] — embeddable HTTP server
//
// Or run the standalone server:
//
//	krsec -config config.yaml
package koreasecurities
