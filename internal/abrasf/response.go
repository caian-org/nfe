package abrasf

import (
	"bytes"
	"encoding/xml"
	"html"
	"strings"
)

// MensagemRetorno mirrors the ABRASF error structure surfaced in any
// ListaMensagemRetorno block.
type MensagemRetorno struct {
	XMLName  xml.Name `xml:"MensagemRetorno"`
	Codigo   string   `xml:"Codigo"`
	Mensagem string   `xml:"Mensagem"`
	Correcao string   `xml:"Correcao"`
}

// ResponsePayload is the unmarshaled shape used by every command to surface
// either success data or returned error messages.
type ResponsePayload struct {
	Mensagens []MensagemRetorno
	// Raw is the XML fragment extracted from the SOAP body, with HTML entities
	// already decoded. Callers needing fine-grained data can re-parse this.
	Raw []byte
}

// HasErrors reports whether the response carries any MensagemRetorno entry.
func (r *ResponsePayload) HasErrors() bool {
	return len(r.Mensagens) > 0
}

// ParseResponse unwraps the SOAP-Body inner XML (often wrapped in an
// <outputXML> element with HTML-entity-encoded content) and pulls out any
// MensagemRetorno entries. It always returns a payload; the Raw field carries
// the decoded inner XML so callers can keep parsing.
func ParseResponse(bodyInner []byte) (*ResponsePayload, error) {
	// Unwrap optional outer wrappers like <SomethingResposta><outputXML>...</outputXML></SomethingResposta>
	raw := bytes.TrimSpace(bodyInner)
	if inner := extractInnerText(raw, "outputXML"); inner != nil {
		raw = inner
	} else if inner := extractInnerText(raw, "return"); inner != nil {
		raw = inner
	}

	// The wrapper's content is typically HTML-entity-encoded (the SOAP layer
	// escaped < and > before sending). Decode if it looks encoded.
	decoded := raw
	if bytes.Contains(raw, []byte("&lt;")) || bytes.Contains(raw, []byte("&amp;")) {
		decoded = []byte(html.UnescapeString(string(raw)))
	}

	payload := &ResponsePayload{Raw: decoded}

	// Find all MensagemRetorno entries — they can appear at any depth.
	for _, frag := range findElements(decoded, "MensagemRetorno") {
		var m MensagemRetorno
		if err := xml.Unmarshal(frag, &m); err == nil {
			payload.Mensagens = append(payload.Mensagens, m)
		}
	}
	return payload, nil
}

// extractInnerText returns the bytes between the opening and closing tag of
// the named element, or nil if the tag is not present.
func extractInnerText(b []byte, name string) []byte {
	openPrefix := []byte("<" + name)
	openIdx := bytes.Index(b, openPrefix)
	if openIdx < 0 {
		return nil
	}
	// Skip attributes and the closing '>' of the opening tag.
	close := bytes.IndexByte(b[openIdx:], '>')
	if close < 0 {
		return nil
	}
	start := openIdx + close + 1
	endTag := []byte("</" + name + ">")
	endIdx := bytes.Index(b[start:], endTag)
	if endIdx < 0 {
		return nil
	}
	return bytes.TrimSpace(b[start : start+endIdx])
}

// findElements returns each XML fragment whose root element matches name. The
// implementation is intentionally string-based so it works with the partial
// fragments ABRASF returns without a single shared root.
func findElements(b []byte, name string) [][]byte {
	var out [][]byte
	rest := b
	open := []byte("<" + name)
	closeTag := []byte("</" + name + ">")
	for {
		idx := bytes.Index(rest, open)
		if idx < 0 {
			return out
		}
		// Ensure the match is a full tag (next byte is space, >, or /).
		next := rest[idx+len(open)]
		if next != ' ' && next != '>' && next != '/' && next != '\t' && next != '\n' && next != '\r' {
			rest = rest[idx+len(open):]
			continue
		}
		end := bytes.Index(rest[idx:], closeTag)
		if end < 0 {
			return out
		}
		fragment := rest[idx : idx+end+len(closeTag)]
		out = append(out, fragment)
		rest = rest[idx+end+len(closeTag):]
	}
}

// FindNFSeNumero scans the raw response for the first <Numero> appearing
// inside an <Nfse><InfNfse> path. Returns empty when not found. Used by emit
// to surface the authorized NFS-e number in the renderer.
func FindNFSeNumero(raw []byte) string {
	inf := extractInnerText(raw, "InfNfse")
	if inf == nil {
		return ""
	}
	n := extractInnerText(inf, "Numero")
	if n == nil {
		return ""
	}
	return strings.TrimSpace(string(n))
}
