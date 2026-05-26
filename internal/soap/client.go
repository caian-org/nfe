// Package soap implements a minimal SOAP 1.1 client tailored for the ABRASF
// NFS-e web service. It builds the envelope and method wrapper manually
// because the WSDL surface in scope is tiny and stable.
package soap

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Cabecalho is the constant ABRASF v2.04 header sent inside nfseCabecMsg with
// every operation.
const Cabecalho = `<?xml version="1.0" encoding="UTF-8"?><cabecalho xmlns="http://www.abrasf.org.br/nfse.xsd" versao="2.04"><versaoDados>2.04</versaoDados></cabecalho>`

// BasicAuth carries optional HTTP basic-auth credentials.
type BasicAuth struct {
	Username, Password string
}

// Options configures a Client.
type Options struct {
	// Endpoint is the SOAP POST URL. When empty, NewClient discovers it from
	// the WSDL at WSDLURL.
	Endpoint string
	// WSDLURL, when set and Endpoint is empty, is fetched at NewClient time
	// and parsed to discover the endpoint URL, the target namespace, and the
	// per-operation soapAction strings.
	WSDLURL            string
	TLSCert            *tls.Certificate
	BasicAuth          *BasicAuth
	InsecureSkipVerify bool          // Mirrors `rejectUnauthorized: false` in the TS source for homologação.
	Timeout            time.Duration // Defaults to 60s when zero.
	NSPrefix           string        // Override the prefix on the method element. Defaults to "tns".
	// MethodNamespace overrides the xmlns of the method body wrapper. When
	// empty and a WSDL was fetched, the WSDL's targetNamespace is used.
	MethodNamespace string
	// SoapActions, when non-nil, overrides per-operation soapAction values
	// (e.g. {"GerarNfse": "nfs#GerarNfse"}). When nil, soapActions discovered
	// from the WSDL are used; if none, the bare operation name is sent.
	SoapActions map[string]string
	// RequestSuffix is appended to the operation name to produce the SOAP
	// body wrapper element name (e.g. "GerarNfse" + "Request" =
	// "<GerarNfseRequest>"). Defaults to "Request" — that's what every ABRASF
	// WSDL I've seen uses.
	RequestSuffix string
}

// Client posts SOAP requests to the configured endpoint.
type Client struct {
	httpClient    *http.Client
	endpoint      string
	basicAuth     *BasicAuth
	nsPrefix      string
	methodNS      string
	soapActions   map[string]string
	requestSuffix string
}

// NewClient constructs a Client. Either Endpoint or WSDLURL must be set:
// Endpoint is used as-is (with optional MethodNamespace + SoapActions
// overrides); WSDLURL is fetched and parsed at NewClient time to populate
// the endpoint, target namespace, and per-operation soapAction values.
func NewClient(opts Options) (*Client, error) {
	if opts.Endpoint == "" && opts.WSDLURL == "" {
		return nil, errors.New("soap: Endpoint ou WSDLURL é obrigatório")
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: opts.InsecureSkipVerify, //nolint:gosec — wired to user config
	}
	if opts.TLSCert != nil {
		tlsCfg.Certificates = []tls.Certificate{*opts.TLSCert}
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	endpoint := opts.Endpoint
	methodNS := opts.MethodNamespace
	soapActions := opts.SoapActions

	if opts.Endpoint == "" {
		info, err := FetchWSDL(context.Background(), opts.WSDLURL, tlsCfg, timeout)
		if err != nil {
			return nil, err
		}
		endpoint = info.Endpoint
		if methodNS == "" {
			methodNS = info.TargetNamespace
		}
		if soapActions == nil {
			soapActions = info.SoapActions
		}
	}

	prefix := opts.NSPrefix
	if prefix == "" {
		prefix = "tns"
	}
	if methodNS == "" {
		methodNS = endpoint
	}
	requestSuffix := opts.RequestSuffix
	if requestSuffix == "" {
		requestSuffix = "Request"
	}

	return &Client{
		httpClient:    &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}, Timeout: timeout},
		endpoint:      endpoint,
		basicAuth:     opts.BasicAuth,
		nsPrefix:      prefix,
		methodNS:      methodNS,
		soapActions:   soapActions,
		requestSuffix: requestSuffix,
	}, nil
}

// Call invokes a SOAP method with the given ABRASF dados message. The
// envelope is constructed as:
//
//	<soap:Envelope ...>
//	  <soap:Body>
//	    <abr:{action} xmlns:abr="...">
//	      <nfseCabecMsg><![CDATA[...header...]]></nfseCabecMsg>
//	      <nfseDadosMsg><![CDATA[...dados...]]></nfseDadosMsg>
//	    </abr:{action}>
//	  </soap:Body>
//	</soap:Envelope>
//
// It returns the response body bytes (the full SOAP envelope from the server).
func (c *Client) Call(ctx context.Context, action string, dados []byte) ([]byte, error) {
	envelope := c.buildEnvelope(action, dados)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(envelope))
	if err != nil {
		return nil, fmt.Errorf("soap: construir requisição: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", c.soapActionFor(action))
	req.Header.Set("Accept", "text/xml")
	if c.basicAuth != nil {
		req.SetBasicAuth(c.basicAuth.Username, c.basicAuth.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("soap: requisição: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("soap: ler resposta: %w", err)
	}

	// SOAP faults arrive with a 5xx status and a SOAP-Fault body; surface both.
	if resp.StatusCode >= 400 {
		return body, fmt.Errorf("soap: servidor retornou HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// buildEnvelope assembles the SOAP 1.1 envelope around the cabecalho + dados.
// The body wrapper element is `<prefix:{action}{requestSuffix}>` to match
// what the ABRASF WSDLs declare (e.g. ConsultarNfseServicoPrestadoRequest).
func (c *Client) buildEnvelope(action string, dados []byte) []byte {
	wrapper := action + c.requestSuffix
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:`)
	b.WriteString(c.nsPrefix)
	b.WriteString(`="`)
	b.WriteString(c.methodNS)
	b.WriteString(`"><soap:Body><`)
	b.WriteString(c.nsPrefix)
	b.WriteRune(':')
	b.WriteString(wrapper)
	b.WriteString(`><nfseCabecMsg><![CDATA[`)
	b.WriteString(Cabecalho)
	b.WriteString(`]]></nfseCabecMsg><nfseDadosMsg><![CDATA[`)
	b.Write(dados)
	b.WriteString(`]]></nfseDadosMsg></`)
	b.WriteString(c.nsPrefix)
	b.WriteRune(':')
	b.WriteString(wrapper)
	b.WriteString(`></soap:Body></soap:Envelope>`)
	return b.Bytes()
}

// soapActionFor returns the SOAPAction header value for a given operation.
// If the WSDL (or Options.SoapActions) supplied one, that is used verbatim
// inside double quotes. Otherwise the bare operation name is sent in quotes.
func (c *Client) soapActionFor(op string) string {
	v, ok := c.soapActions[op]
	if !ok || v == "" {
		v = op
	}
	return `"` + v + `"`
}

// ExtractBody parses a SOAP response envelope and returns the inner content
// of the soap:Body. The Body normally contains a single child element whose
// content is the operation's typed return value.
func ExtractBody(envelope []byte) ([]byte, error) {
	type body struct {
		InnerXML []byte `xml:",innerxml"`
	}
	type env struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    body     `xml:"Body"`
	}
	var e env
	dec := xml.NewDecoder(bytes.NewReader(envelope))
	if err := dec.Decode(&e); err != nil {
		return nil, fmt.Errorf("soap: decodificar envelope: %w", err)
	}
	return bytes.TrimSpace(e.Body.InnerXML), nil
}
