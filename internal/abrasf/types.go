package abrasf

import "encoding/xml"

// Enum constants — match the integer values used in the ABRASF schema.
const (
	TipoRPS               = 1
	StatusRPSNormal       = 1
	StatusRPSCancelado    = 2
	ExigibilidadeExigivel = 1

	ISSRetidoSim = 1
	ISSRetidoNao = 2

	OptanteSimplesNao    = 2
	IncentivadorCultNao  = 2
	RegimeEspecialPadrao = 1 // ResponsavelRetencao + RegimeEspecialTributacao are hardcoded to 1 by the TS source.
)

// CpfCnpj is a discriminated identifier; exactly one of CPF/CNPJ is set.
type CpfCnpj struct {
	CNPJ string `xml:"Cnpj,omitempty"`
	CPF  string `xml:"Cpf,omitempty"`
}

// Endereco is the XML address block used both inside Prestador and inside
// TomadorServico.
type Endereco struct {
	XMLName         xml.Name `xml:"Endereco"`
	Endereco        string   `xml:"Endereco"`
	Numero          string   `xml:"Numero"`
	Complemento     string   `xml:"Complemento,omitempty"`
	Bairro          string   `xml:"Bairro"`
	CodigoMunicipio string   `xml:"CodigoMunicipio"`
	UF              string   `xml:"Uf"`
	CEP             string   `xml:"Cep"`
}

// Contato is optional and only emitted when at least one field is set.
type Contato struct {
	XMLName  xml.Name `xml:"Contato"`
	Telefone string   `xml:"Telefone,omitempty"`
	Email    string   `xml:"Email,omitempty"`
}

// Prestador appears in the RPS body, in queries, and in cancellation
// pedidos. The schema permits only CNPJ identification.
type Prestador struct {
	XMLName            xml.Name `xml:"Prestador"`
	CpfCnpj            CpfCnpj  `xml:"CpfCnpj"`
	InscricaoMunicipal string   `xml:"InscricaoMunicipal"`
}

// IdentificacaoRps is the RPS identifier used in both emit and per-RPS query.
type IdentificacaoRps struct {
	XMLName xml.Name `xml:"IdentificacaoRps"`
	Numero  int      `xml:"Numero"`
	Serie   string   `xml:"Serie"`
	Tipo    int      `xml:"Tipo"`
}

// RpsHead carries Identificacao + emission metadata.
type RpsHead struct {
	XMLName          xml.Name         `xml:"Rps"`
	IdentificacaoRps IdentificacaoRps `xml:"IdentificacaoRps"`
	DataEmissao      string           `xml:"DataEmissao"`
	Status           int              `xml:"Status"`
}

// Valores is the monetary section of Servico. Field declaration order maps
// directly to the XSD-required element order.
type Valores struct {
	XMLName                xml.Name `xml:"Valores"`
	ValorServicos          Dec2     `xml:"ValorServicos"`
	ValorDeducoes          Dec2     `xml:"ValorDeducoes"`
	ValorPis               Dec2     `xml:"ValorPis"`
	ValorCofins            Dec2     `xml:"ValorCofins"`
	ValorInss              Dec2     `xml:"ValorInss"`
	ValorIr                Dec2     `xml:"ValorIr"`
	ValorCsll              Dec2     `xml:"ValorCsll"`
	OutrasRetencoes        Dec2     `xml:"OutrasRetencoes"`
	ValTotTributos         Dec2     `xml:"ValTotTributos"`
	ValorIss               *Dec2    `xml:"ValorIss,omitempty"`
	Aliquota               *Dec1    `xml:"Aliquota,omitempty"`
	DescontoIncondicionado Dec2     `xml:"DescontoIncondicionado"`
	DescontoCondicionado   Dec2     `xml:"DescontoCondicionado"`
}

// Servico bundles the value group with the service identification fields.
type Servico struct {
	XMLName             xml.Name `xml:"Servico"`
	Valores             Valores  `xml:"Valores"`
	IssRetido           int      `xml:"IssRetido"`
	ResponsavelRetencao int      `xml:"ResponsavelRetencao"` // Hardcoded to 1 by TS.
	ItemListaServico    string   `xml:"ItemListaServico"`
	CodigoCnae          string   `xml:"CodigoCnae,omitempty"`
	Discriminacao       string   `xml:"Discriminacao"`
	CodigoMunicipio     string   `xml:"CodigoMunicipio"`
	CodigoPais          string   `xml:"CodigoPais"` // TS hardcodes "1058".
	ExigibilidadeISS    int      `xml:"ExigibilidadeISS"`
	MunicipioIncidencia string   `xml:"MunicipioIncidencia"`
}

// TomadorServico is the service buyer. CpfCnpj may be absent when the buyer
// is a foreign individual without local documents — the schema makes it
// optional, but Validate refuses notas without one or the other.
type TomadorServico struct {
	XMLName                xml.Name                `xml:"TomadorServico"`
	IdentificacaoTomador   *IdentificacaoTomador   `xml:"IdentificacaoTomador,omitempty"`
	RazaoSocial            string                  `xml:"RazaoSocial,omitempty"`
	Endereco               *Endereco               `xml:"Endereco,omitempty"`
	Contato                *Contato                `xml:"Contato,omitempty"`
}

// IdentificacaoTomador wraps the CPF/CNPJ in its own element so the schema's
// discriminated-union semantics are honored.
type IdentificacaoTomador struct {
	XMLName xml.Name `xml:"IdentificacaoTomador"`
	CpfCnpj CpfCnpj  `xml:"CpfCnpj"`
}

// InfDeclaracaoPrestacaoServico is the signed inner element of a GerarNfse
// envelope. Its Id attribute is the URI referenced by the XMLDSig Reference.
type InfDeclaracaoPrestacaoServico struct {
	XMLName                   xml.Name        `xml:"InfDeclaracaoPrestacaoServico"`
	ID                        string          `xml:"Id,attr"`
	Rps                       RpsHead         `xml:"Rps"`
	Competencia               string          `xml:"Competencia"`
	Servico                   Servico         `xml:"Servico"`
	Prestador                 Prestador       `xml:"Prestador"`
	TomadorServico            *TomadorServico `xml:"TomadorServico,omitempty"`
	RegimeEspecialTributacao  int             `xml:"RegimeEspecialTributacao"`
	OptanteSimplesNacional    int             `xml:"OptanteSimplesNacional"`
	IncentivoFiscal           int             `xml:"IncentivoFiscal"`
	InformacoesComplementares string          `xml:"InformacoesComplementares,omitempty"`
}

// RpsContainer is the <Rps> wrapper around InfDeclaracaoPrestacaoServico.
// The signature ends up appended here, as last child of the container.
type RpsContainer struct {
	XMLName xml.Name                      `xml:"Rps"`
	Inf     InfDeclaracaoPrestacaoServico `xml:"InfDeclaracaoPrestacaoServico"`
}

// GerarNfseEnvio is the top-level emission envelope. The xmlns attribute is
// declared explicitly so the WS sees the expected default namespace without
// Go's xml package injecting a prefixed declaration.
type GerarNfseEnvio struct {
	XMLName xml.Name     `xml:"GerarNfseEnvio"`
	Xmlns   string       `xml:"xmlns,attr"`
	Rps     RpsContainer `xml:"Rps"`
}

// ConsultarNfseServicoPrestadoEnvio is used by the query command for both
// number-based and date-range lookups.
type ConsultarNfseServicoPrestadoEnvio struct {
	XMLName            xml.Name            `xml:"ConsultarNfseServicoPrestadoEnvio"`
	Xmlns              string              `xml:"xmlns,attr"`
	Prestador          Prestador           `xml:"Prestador"`
	NumeroNfse         string              `xml:"NumeroNfse,omitempty"`
	PeriodoCompetencia *PeriodoCompetencia `xml:"PeriodoCompetencia,omitempty"`
	Pagina             int                 `xml:"Pagina"`
}

// PeriodoCompetencia is the date-range filter used by query.
type PeriodoCompetencia struct {
	XMLName     xml.Name `xml:"PeriodoCompetencia"`
	DataInicial string   `xml:"DataInicial"`
	DataFinal   string   `xml:"DataFinal"`
}

// IdentificacaoNfse identifies a previously authorized NFS-e for cancellation.
type IdentificacaoNfse struct {
	XMLName            xml.Name `xml:"IdentificacaoNfse"`
	Numero             string   `xml:"Numero"`
	CpfCnpj            CpfCnpj  `xml:"CpfCnpj"`
	InscricaoMunicipal string   `xml:"InscricaoMunicipal"`
	CodigoMunicipio    string   `xml:"CodigoMunicipio"`
}

// InfPedidoCancelamento is the signed inner element of a cancellation envelope.
type InfPedidoCancelamento struct {
	XMLName            xml.Name          `xml:"InfPedidoCancelamento"`
	ID                 string            `xml:"Id,attr,omitempty"`
	IdentificacaoNfse  IdentificacaoNfse `xml:"IdentificacaoNfse"`
	CodigoCancelamento int               `xml:"CodigoCancelamento"`
}

// PedidoContainer is the <Pedido> wrapper around InfPedidoCancelamento, the
// element to which the XMLDSig Signature is appended as last child.
type PedidoContainer struct {
	XMLName xml.Name              `xml:"Pedido"`
	Inf     InfPedidoCancelamento `xml:"InfPedidoCancelamento"`
}

// CancelarNfseEnvio is the cancellation envelope.
type CancelarNfseEnvio struct {
	XMLName xml.Name        `xml:"CancelarNfseEnvio"`
	Xmlns   string          `xml:"xmlns,attr"`
	Pedido  PedidoContainer `xml:"Pedido"`
}
