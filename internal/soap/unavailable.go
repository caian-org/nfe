package soap

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// UnavailableError marks transport-level failures (DNS, connection refused,
// gateway errors) that mean the municipal webservice cannot be reached, as
// opposed to a business rejection or a malformed payload.
type UnavailableError struct {
	URL    string
	Status int   // 0 when the failure happened before an HTTP response
	Err    error // underlying error, kept for errors.Is / errors.As
}

func (e *UnavailableError) Error() string {
	reason := ""
	switch {
	case e.Status != 0:
		reason = fmt.Sprintf("HTTP %d", e.Status)
	case e.Err != nil:
		reason = e.Err.Error()
	}
	return fmt.Sprintf("soap: webservice municipal indisponível em %s (%s) — confira a página de integração da prefeitura e os campos soap.wsdl_*/soap.endpoint_* do config.toml", e.URL, reason)
}

func (e *UnavailableError) Unwrap() error { return e.Err }

// classifyTransportErr wraps DNS and dial-level failures in UnavailableError
// and returns other errors with the usual soap prefix.
func classifyTransportErr(prefix, url string, err error) error {
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || errors.As(err, &opErr) || errors.Is(err, context.DeadlineExceeded) {
		return &UnavailableError{URL: url, Err: err}
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func isGatewayStatus(status int) bool {
	return status == 502 || status == 503 || status == 504
}
