package ali

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestAliResponseAcceptsNumericErrorCode(t *testing.T) {
	t.Parallel()

	var resp AliResponse
	body := []byte(`{"code":400,"message":"bad request","request_id":"req-1","output":{"code":123,"message":"output error","results":[{"code":456,"message":"result error"}]}}`)

	if err := common.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got, want := string(resp.Code), "400"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if got, want := string(resp.Output.Code), "123"; got != want {
		t.Fatalf("output.code = %q, want %q", got, want)
	}
	if got, want := string(resp.Output.Results[0].Code), "456"; got != want {
		t.Fatalf("results[0].code = %q, want %q", got, want)
	}
}
