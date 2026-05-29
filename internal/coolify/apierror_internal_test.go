package coolify

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func responseWithBody(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewAPIError_RedactsBearerToken(t *testing.T) {
	const token = "abc123def456_token.value-XYZ"
	const cfSecret = "cf-secret-7f3a9b2c1d"
	body := "unauthorized: header was Authorization: Bearer " + token +
		" and CF-Access-Client-Secret: " + cfSecret

	resp := responseWithBody(body)
	defer func() { _ = resp.Body.Close() }()
	err := newAPIError(resp)

	if strings.Contains(err.Body, token) {
		t.Errorf("APIError.Body leaked the Bearer token:\n%s", err.Body)
	}
	if strings.Contains(err.Body, cfSecret) {
		t.Errorf("APIError.Body leaked the CF Access secret:\n%s", err.Body)
	}
	if !strings.Contains(err.Body, "Bearer [REDACTED]") {
		t.Errorf("APIError.Body should mark the redacted token, got:\n%s", err.Body)
	}
	if !strings.Contains(err.Body, "CF-Access-Client-Secret: [REDACTED]") {
		t.Errorf("APIError.Body should mark the redacted CF secret, got:\n%s", err.Body)
	}
	// Error() must not leak either, since it embeds the body.
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), cfSecret) {
		t.Errorf("APIError.Error() leaked a credential:\n%s", err.Error())
	}
}

func TestNewAPIError_LeavesCleanBodyIntact(t *testing.T) {
	body := `{"message":"validation failed: name is required"}`
	resp := responseWithBody(body)
	defer func() { _ = resp.Body.Close() }()
	if got := newAPIError(resp).Body; got != body {
		t.Errorf("clean body must pass through unchanged:\ngot:  %s\nwant: %s", got, body)
	}
}
