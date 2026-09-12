package specs_test

import (
	"testing"

	"github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

func TestRetiredAPIsPreserveTypesWithoutActiveRegistration(t *testing.T) {
	// Exercise the public names from an external package: source compatibility
	// must survive regeneration, while new documented calls must not use them.
	for _, tc := range []struct {
		path     string
		apiID    string
		request  any
		response any
	}{
		{"/api/dostk/mrkcond", specs.KiwoomAPIIDKa10087,
			specs.KiwoomApiDostkMrkcondKa10087Request{StkCd: "005930"},
			specs.KiwoomApiDostkMrkcondKa10087Response{BidReqBaseTm: "160000"}},
		{"/api/dostk/rkinfo", specs.KiwoomAPIIDKa10098,
			specs.KiwoomApiDostkRkinfoKa10098Request{MrktTp: "000"},
			specs.KiwoomApiDostkRkinfoKa10098Response{OvtSigpricFluRtRank: []specs.KiwoomApiDostkRkinfoKa10098ResponseItem{{StkCd: "005930"}}}},
	} {
		t.Run(tc.apiID, func(t *testing.T) {
			if tc.request == nil || tc.response == nil {
				t.Fatal("missing legacy payload type")
			}
			if _, ok := specs.LookupDocumentedEndpointSpec(tc.path, tc.apiID); ok {
				t.Fatal("retired API is still active")
			}
			if specs.NewDocumentedEndpointRequest(tc.path, tc.apiID) != nil || specs.NewDocumentedEndpointResponse(tc.path, tc.apiID) != nil {
				t.Fatal("retired API still has an active factory")
			}
		})
	}
}
