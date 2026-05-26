package soap

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WSDLInfo carries the subset of WSDL data we need to build a proper SOAP
// request: the document/literal endpoint URL, the target namespace (used as
// the xmlns of the method element), and the soapAction string of each
// operation (e.g. "nfs#ConsultarNfseServicoPrestado").
type WSDLInfo struct {
	Endpoint        string
	TargetNamespace string
	// SoapActions maps operation name -> soapAction value.
	SoapActions map[string]string
}

// ActionFor returns the soapAction for op, or a sensible default if the WSDL
// didn't carry one for that operation.
func (w *WSDLInfo) ActionFor(op string) string {
	if w == nil || w.SoapActions == nil {
		return op
	}
	if a, ok := w.SoapActions[op]; ok {
		return a
	}
	return op
}

// FetchWSDL retrieves and parses the WSDL at url. The fetch honors the
// supplied tls.Config (so the same mTLS cert can be reused) and uses the
// supplied timeout.
func FetchWSDL(ctx context.Context, url string, tlsCfg *tls.Config, timeout time.Duration) (*WSDLInfo, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("wsdl: construir requisição: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wsdl: buscar %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wsdl: buscar %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wsdl: ler resposta: %w", err)
	}
	return ParseWSDL(body)
}

// ParseWSDL extracts WSDLInfo from a definitions document. It looks for the
// soap address inside the service/port and the soapAction inside each
// operation binding. The structure is namespace-prefix-agnostic so it
// tolerates both `soap:` and `wsdl:` styles.
func ParseWSDL(b []byte) (*WSDLInfo, error) {
	type addr struct {
		Location string `xml:"location,attr"`
	}
	type op struct {
		Name       string `xml:"name,attr"`
		SoapAction string `xml:"soapAction,attr"`
	}
	type wopBinding struct {
		Name string `xml:"name,attr"`
		Op   addr   `xml:"operation"` // we look up soapAction separately
	}
	// Use a recursive walker because xml.Unmarshal is fiddly with mixed
	// prefixes. Decode token-by-token.
	info := &WSDLInfo{SoapActions: map[string]string{}}

	dec := xml.NewDecoder(bytesReader(b))
	dec.Strict = false

	// The WSDL nests <soap:operation soapAction="..."/> inside
	// <wsdl:operation name="..."> inside <wsdl:binding>. To pair them we track
	// the current operation name and, whenever we see a SOAP-namespaced
	// `operation` child carrying a soapAction, record the binding.
	var currentOp string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("wsdl: parse: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "definitions":
			for _, a := range se.Attr {
				if a.Name.Local == "targetNamespace" {
					info.TargetNamespace = a.Value
				}
			}
		case "address":
			for _, a := range se.Attr {
				if a.Name.Local == "location" && info.Endpoint == "" {
					info.Endpoint = a.Value
				}
			}
		case "operation":
			var name, action string
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "name":
					name = a.Value
				case "soapAction":
					action = a.Value
				}
			}
			switch {
			case action != "":
				// <soap:operation soapAction="..."/> — pair with the enclosing
				// <wsdl:operation>'s name.
				if currentOp != "" {
					info.SoapActions[currentOp] = action
				}
			case name != "":
				currentOp = name
			}
		}
	}

	if info.Endpoint == "" {
		return nil, fmt.Errorf("wsdl: location de soap:address não encontrada")
	}
	if info.TargetNamespace == "" {
		return nil, fmt.Errorf("wsdl: targetNamespace não encontrado")
	}
	return info, nil
}

// bytesReader is a one-line wrapper so this file doesn't have to import
// "bytes" just for a *bytes.Reader.
func bytesReader(b []byte) *bytesReaderImpl { return &bytesReaderImpl{b: b} }

type bytesReaderImpl struct {
	b []byte
	i int
}

func (r *bytesReaderImpl) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
