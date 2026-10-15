package dps

import (
	"encoding/xml"
	"strconv"
)

// Namespace is the Sistema Nacional NFS-e XML namespace.
const Namespace = "http://www.sped.fazenda.gov.br/nfse"

// Versao is the DPS layout version this package produces.
const Versao = "1.01"

// Enumerations used by the builder. Values follow Anexo I of the national layout.
const (
	TpAmbProducao    = 1
	TpAmbHomologacao = 2

	TpEmitPrestador = 1

	TribISSQNTributavel = 1

	TpRetISSQNNaoRetido     = 1
	TpRetISSQNRetidoTomador = 2
	TpRetISSQNRetidoInterm  = 3
	IndTotTribNaoInformado  = 0
	FinNFSeRegular          = 0
	IndDestTomador          = 0
)

// Dec2 renders TSDec15V2 / TSDec1V2 values with exactly two fractional digits.
type Dec2 float64

func (d Dec2) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(strconv.FormatFloat(float64(d), 'f', 2, 64), start)
}

// DPS is the document root.
type DPS struct {
	XMLName xml.Name `xml:"DPS"`
	Xmlns   string   `xml:"xmlns,attr"`
	Versao  string   `xml:"versao,attr"`
	InfDPS  InfDPS   `xml:"infDPS"`
}

// InfDPS is the signed element. Id follows the 45-char rule
// "DPS" + cLocEmi(7) + tpInsc(1) + inscrição(14) + série(5) + nDPS(15).
type InfDPS struct {
	XMLName  xml.Name  `xml:"infDPS"`
	ID       string    `xml:"Id,attr"`
	TpAmb    int       `xml:"tpAmb"`
	DhEmi    string    `xml:"dhEmi"`
	VerAplic string    `xml:"verAplic"`
	Serie    string    `xml:"serie"`
	NDPS     string    `xml:"nDPS"`
	DCompet  string    `xml:"dCompet"`
	TpEmit   int       `xml:"tpEmit"`
	CLocEmi  string    `xml:"cLocEmi"`
	Prest    Prestador `xml:"prest"`
	Toma     *Pessoa   `xml:"toma,omitempty"`
	Serv     Serv      `xml:"serv"`
	Valores  Valores   `xml:"valores"`
	IBSCBS   *IBSCBS   `xml:"IBSCBS,omitempty"`
}

// Prestador identifies the emitter. Cadastral data (name, address) is kept
// by the municipality, so only the identifiers and regime travel.
type Prestador struct {
	XMLName xml.Name `xml:"prest"`
	CNPJ    string   `xml:"CNPJ,omitempty"`
	CPF     string   `xml:"CPF,omitempty"`
	IM      string   `xml:"IM,omitempty"`
	RegTrib RegTrib  `xml:"regTrib"`
}

type RegTrib struct {
	XMLName    xml.Name `xml:"regTrib"`
	OpSimpNac  int      `xml:"opSimpNac"`
	RegEspTrib int      `xml:"regEspTrib"`
}

// Pessoa is TCInfoPessoa (tomador / intermediário).
type Pessoa struct {
	CNPJ  string    `xml:"CNPJ,omitempty"`
	CPF   string    `xml:"CPF,omitempty"`
	XNome string    `xml:"xNome"`
	End   *Endereco `xml:"end,omitempty"`
	Fone  string    `xml:"fone,omitempty"`
	Email string    `xml:"email,omitempty"`
}

type Endereco struct {
	XMLName xml.Name    `xml:"end"`
	EndNac  EnderecoNac `xml:"endNac"`
	XLgr    string      `xml:"xLgr"`
	Nro     string      `xml:"nro"`
	XCpl    string      `xml:"xCpl,omitempty"`
	XBairro string      `xml:"xBairro"`
}

type EnderecoNac struct {
	XMLName xml.Name `xml:"endNac"`
	CMun    string   `xml:"cMun"`
	CEP     string   `xml:"CEP"`
}

type Serv struct {
	XMLName   xml.Name   `xml:"serv"`
	LocPrest  LocPrest   `xml:"locPrest"`
	CServ     CServ      `xml:"cServ"`
	InfoCompl *InfoCompl `xml:"infoCompl,omitempty"`
}

type LocPrest struct {
	XMLName       xml.Name `xml:"locPrest"`
	CLocPrestacao string   `xml:"cLocPrestacao"`
}

type CServ struct {
	XMLName   xml.Name `xml:"cServ"`
	CTribNac  string   `xml:"cTribNac"`
	XDescServ string   `xml:"xDescServ"`
	CNBS      string   `xml:"cNBS,omitempty"`
}

type InfoCompl struct {
	XMLName  xml.Name `xml:"infoCompl"`
	XInfComp string   `xml:"xInfComp,omitempty"`
}

type Valores struct {
	XMLName    xml.Name   `xml:"valores"`
	VServPrest VServPrest `xml:"vServPrest"`
	Trib       Trib       `xml:"trib"`
}

type VServPrest struct {
	XMLName xml.Name `xml:"vServPrest"`
	VServ   Dec2     `xml:"vServ"`
}

type Trib struct {
	XMLName xml.Name `xml:"trib"`
	TribMun TribMun  `xml:"tribMun"`
	TotTrib TotTrib  `xml:"totTrib"`
}

type TribMun struct {
	XMLName    xml.Name `xml:"tribMun"`
	TribISSQN  int      `xml:"tribISSQN"`
	TpRetISSQN int      `xml:"tpRetISSQN"`
	PAliq      *Dec2    `xml:"pAliq,omitempty"`
}

type TotTrib struct {
	XMLName    xml.Name `xml:"totTrib"`
	IndTotTrib int      `xml:"indTotTrib"`
}

// IBSCBS is the reforma tributária group (TCRTCInfoIBSCBS). Only the fields
// the SIGCORP platform requires are modelled.
type IBSCBS struct {
	XMLName xml.Name      `xml:"IBSCBS"`
	FinNFSe int           `xml:"finNFSe"`
	CIndOp  string        `xml:"cIndOp"`
	IndDest int           `xml:"indDest"`
	Valores IBSCBSValores `xml:"valores"`
}

type IBSCBSValores struct {
	XMLName xml.Name   `xml:"valores"`
	Trib    IBSCBSTrib `xml:"trib"`
}

type IBSCBSTrib struct {
	XMLName xml.Name `xml:"trib"`
	GIBSCBS GIBSCBS  `xml:"gIBSCBS"`
}

type GIBSCBS struct {
	XMLName    xml.Name `xml:"gIBSCBS"`
	CST        string   `xml:"CST"`
	CClassTrib string   `xml:"cClassTrib"`
}
