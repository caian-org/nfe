package dps

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strings"
)

// NFSe is the subset of an authorized national NFS-e that the CLI shows.
type NFSe struct {
	Numero        string // nNFSe
	ChaveAcesso   string // infNFSe@Id without the "NFS" prefix (50 digits)
	DataProcesso  string // dhProc
	DataEmissao   string // DPS dhEmi echoed inside the NFS-e
	ValorServicos string // vServ
	Tomador       string // toma/xNome
	NumeroDPS     string // nDPS
}

// ParseNFSe extracts the display fields from a national NFS-e XML.
func ParseNFSe(raw []byte) (*NFSe, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("dps: XML da NFS-e vazio")
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	out := &NFSe{}
	var path []string
	seen := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
			if t.Name.Local == "infNFSe" {
				seen = true
				for _, a := range t.Attr {
					if a.Name.Local == "Id" {
						out.ChaveAcesso = strings.TrimPrefix(a.Value, "NFS")
					}
				}
			}
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		case xml.CharData:
			val := strings.TrimSpace(string(t))
			if val == "" || len(path) == 0 {
				continue
			}
			switch path[len(path)-1] {
			case "nNFSe":
				if out.Numero == "" {
					out.Numero = val
				}
			case "dhProc":
				out.DataProcesso = val
			case "dhEmi":
				if out.DataEmissao == "" {
					out.DataEmissao = val
				}
			case "vServ":
				if out.ValorServicos == "" {
					out.ValorServicos = val
				}
			case "nDPS":
				out.NumeroDPS = val
			case "xNome":
				if len(path) >= 2 && path[len(path)-2] == "toma" {
					out.Tomador = val
				}
			}
		}
	}
	if !seen {
		return nil, errors.New("dps: XML sem elemento infNFSe")
	}
	return out, nil
}
