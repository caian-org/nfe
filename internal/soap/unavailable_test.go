package soap_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/soap"
)

func TestFetchWSDLBadGatewayIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`<h1>Proxy Error</h1>DNS lookup failure for: abrasffrancodarocha.meumunicipio.online`))
	}))
	defer srv.Close()

	_, err := soap.FetchWSDL(context.Background(), srv.URL+"?wsdl", nil, time.Second)
	var ue *soap.UnavailableError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, http.StatusBadGateway, ue.Status)
	assert.Contains(t, err.Error(), "indisponível")
	assert.Contains(t, err.Error(), "HTTP 502")
}

func TestFetchWSDLOtherStatusKeepsPlainError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := soap.FetchWSDL(context.Background(), srv.URL+"?wsdl", nil, time.Second)
	require.Error(t, err)
	var ue *soap.UnavailableError
	assert.False(t, errors.As(err, &ue))
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestNewClientDNSFailureIsUnavailable(t *testing.T) {
	_, err := soap.NewClient(soap.Options{WSDLURL: "https://nfe-nxdomain.invalid/ws?wsdl", Timeout: 3 * time.Second})
	var ue *soap.UnavailableError
	require.ErrorAs(t, err, &ue)
	assert.Contains(t, err.Error(), "nfe-nxdomain.invalid")
}

func TestCallBadGatewayIsUnavailableAndReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("maintenance"))
	}))
	defer srv.Close()
	c, err := soap.NewClient(soap.Options{Endpoint: srv.URL})
	require.NoError(t, err)
	body, err := c.Call(context.Background(), "GerarNfse", []byte("<x/>"))
	var ue *soap.UnavailableError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "maintenance", string(body))
}

func TestCallConnectionRefusedIsUnavailable(t *testing.T) {
	c, err := soap.NewClient(soap.Options{Endpoint: "http://127.0.0.1:1/ws/nfs"})
	require.NoError(t, err)
	_, err = c.Call(context.Background(), "GerarNfse", []byte("<x/>"))
	var ue *soap.UnavailableError
	require.ErrorAs(t, err, &ue)
}

func TestCallInternalServerErrorStaysSOAPFault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<soap:Fault/>`))
	}))
	defer srv.Close()
	c, err := soap.NewClient(soap.Options{Endpoint: srv.URL})
	require.NoError(t, err)
	_, err = c.Call(context.Background(), "GerarNfse", []byte("<x/>"))
	require.Error(t, err)
	var ue *soap.UnavailableError
	assert.False(t, errors.As(err, &ue))
	assert.Contains(t, err.Error(), "HTTP 500")
}
