package soap_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	mathrand "math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/soap"
)

func makeCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(mathrand.Int63()),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost", "127.0.0.1"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{cert.Raw}, PrivateKey: key, Leaf: cert}, pool
}

func TestCallSendsExpectedEnvelope(t *testing.T) {
	var (
		captured     []byte
		gotAction    string
		gotCT        string
		gotBasicAuth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		gotAction = r.Header.Get("SOAPAction")
		gotCT = r.Header.Get("Content-Type")
		gotBasicAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Write([]byte(`<?xml version="1.0"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><GerarNfseResponse><outputXML>&lt;Nfse/&gt;</outputXML></GerarNfseResponse></soap:Body></soap:Envelope>`))
	}))
	defer srv.Close()

	client, err := soap.NewClient(soap.Options{
		Endpoint:  srv.URL,
		BasicAuth: &soap.BasicAuth{Username: "alice", Password: "s3cret"},
	})
	require.NoError(t, err)

	resp, err := client.Call(context.Background(), "GerarNfse", []byte(`<GerarNfseEnvio>x</GerarNfseEnvio>`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), "GerarNfseResponse")

	envelope := string(captured)
	assert.Contains(t, envelope, "<soap:Envelope")
	assert.Contains(t, envelope, "xmlns:soap=\"http://schemas.xmlsoap.org/soap/envelope/\"")
	assert.Contains(t, envelope, "<tns:GerarNfseRequest>")
	assert.Contains(t, envelope, "</tns:GerarNfseRequest>")
	assert.Contains(t, envelope, "<nfseCabecMsg><![CDATA[<?xml")
	assert.Contains(t, envelope, "versao=\"2.04\"")
	assert.Contains(t, envelope, "<nfseDadosMsg><![CDATA[<GerarNfseEnvio>x</GerarNfseEnvio>]]></nfseDadosMsg>")
	assert.Equal(t, `"GerarNfse"`, gotAction)
	assert.Equal(t, "text/xml; charset=utf-8", gotCT)
	assert.NotEmpty(t, gotBasicAuth, "Authorization header must be set when BasicAuth is configured")
}

func TestCallEmbedsDadosVerbatim(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Write([]byte("<env/>"))
	}))
	defer srv.Close()

	client, err := soap.NewClient(soap.Options{Endpoint: srv.URL})
	require.NoError(t, err)

	dados := []byte(`<GerarNfseEnvio xmlns="http://www.abrasf.org.br/nfse.xsd">x</GerarNfseEnvio>`)
	_, err = client.Call(context.Background(), "GerarNfse", dados)
	require.NoError(t, err)
	assert.Contains(t, string(captured), `<nfseDadosMsg><![CDATA[<GerarNfseEnvio xmlns="http://www.abrasf.org.br/nfse.xsd">x</GerarNfseEnvio>]]></nfseDadosMsg>`)
}

func TestCallUsesExplicitEndpointAsIs(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Write([]byte("<env/>"))
	}))
	defer srv.Close()

	// When Endpoint is set explicitly, NewClient must use it verbatim — the
	// caller has already pointed at the SOAP URL (not the WSDL URL).
	client, err := soap.NewClient(soap.Options{Endpoint: srv.URL + "/abrasf/ws/nfs"})
	require.NoError(t, err)
	_, err = client.Call(context.Background(), "GerarNfse", []byte("<x/>"))
	require.NoError(t, err)
	assert.Equal(t, "/abrasf/ws/nfs", gotPath)
}

func TestNewClientFetchesWSDL(t *testing.T) {
	// A fake WSDL server that returns a minimal but realistic ABRASF binding.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.RequestURI(), "?wsdl") {
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/" xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/" targetNamespace="http://example.com/svc">
  <binding name="B" type="P">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="GerarNfse"><soap:operation soapAction="nfs#GerarNfse"/></operation>
    <operation name="ConsultarNfseServicoPrestado"><soap:operation soapAction="nfs#ConsultarNfseServicoPrestado"/></operation>
  </binding>
  <service name="S">
    <port name="P" binding="tns:B"><soap:address location="` + getSOAPURL(r) + `/svc"/></port>
  </service>
</definitions>`))
			return
		}
		// Echo back the captured request for the test to assert.
		w.Write([]byte("<env/>"))
	}))
	defer srv.Close()

	client, err := soap.NewClient(soap.Options{WSDLURL: srv.URL + "/svc?wsdl"})
	require.NoError(t, err)
	_, err = client.Call(context.Background(), "ConsultarNfseServicoPrestado", []byte("<x/>"))
	require.NoError(t, err)
}

// getSOAPURL returns the test server's URL based on the inbound request.
func getSOAPURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func TestCallSurfacesHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<soap:Fault><faultstring>boom</faultstring></soap:Fault>`))
	}))
	defer srv.Close()

	client, err := soap.NewClient(soap.Options{Endpoint: srv.URL})
	require.NoError(t, err)
	body, err := client.Call(context.Background(), "GerarNfse", []byte("<x/>"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
	// Body must still be returned so the caller can inspect the SOAP Fault.
	assert.Contains(t, string(body), "boom")
}

func TestCallHonorsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	client, err := soap.NewClient(soap.Options{Endpoint: srv.URL, Timeout: time.Hour})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = client.Call(ctx, "GerarNfse", []byte("<x/>"))
	require.Error(t, err)
}

func TestNewClientRejectsEmptyEndpoint(t *testing.T) {
	_, err := soap.NewClient(soap.Options{})
	require.Error(t, err)
}

func TestMTLSRoundTrip(t *testing.T) {
	// Generate a server cert and a separate client cert.
	serverCert, _ := makeCert(t)
	clientCert, clientCAPool := makeCert(t)

	var receivedClient bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) > 0 {
			receivedClient = true
		}
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte("<env/>"))
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    clientCAPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	srv.StartTLS()
	defer srv.Close()

	client, err := soap.NewClient(soap.Options{
		Endpoint:           srv.URL,
		TLSCert:            &clientCert,
		InsecureSkipVerify: true, // server cert is self-signed
	})
	require.NoError(t, err)

	_, err = client.Call(context.Background(), "GerarNfse", []byte("<x/>"))
	require.NoError(t, err)
	assert.True(t, receivedClient, "server must have received the client certificate via mTLS")
}

func TestExtractBody(t *testing.T) {
	env := []byte(`<?xml version="1.0"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <GerarNfseResponse><outputXML>&lt;Nfse/&gt;</outputXML></GerarNfseResponse>
  </soap:Body>
</soap:Envelope>`)

	body, err := soap.ExtractBody(env)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(body), "GerarNfseResponse"),
		"body must contain the operation response: %s", body)
}

func TestExtractBodyMalformed(t *testing.T) {
	_, err := soap.ExtractBody([]byte(`not xml at all`))
	require.Error(t, err)
}
