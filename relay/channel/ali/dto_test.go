package ali

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestAliResponseAcceptsNumericErrorCodes(t *testing.T) {
	body := []byte(`{
		"code": 500,
		"message": "top level failed",
		"output": {
			"code": 400,
			"message": "output failed",
			"results": [
				{"code": 12345, "message": "result failed"}
			]
		}
	}`)

	var response AliResponse
	if err := common.Unmarshal(body, &response); err != nil {
		t.Fatalf("common.Unmarshal() error = %v", err)
	}

	if string(response.Code) != "500" {
		t.Fatalf("response.Code = %q, want 500", response.Code)
	}
	if string(response.Output.Code) != "400" {
		t.Fatalf("response.Output.Code = %q, want 400", response.Output.Code)
	}
	if string(response.Output.Results[0].Code) != "12345" {
		t.Fatalf("response.Output.Results[0].Code = %q, want 12345", response.Output.Results[0].Code)
	}
}
