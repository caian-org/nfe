package abrasf

import (
	"strconv"
	"time"

	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/nota"
)

// BuildRPS maps a configured prestador and a per-invoice input into the wire
// types that BuildGerarNfse later serializes. The function is a Go port of
// nfse-service.createRPSFromInput in the original TypeScript source.
//
// today is injected so tests can pin DataEmissao/Competencia to a fixed date;
// passing zero defaults to time.Now.
func BuildRPS(cfg *config.Config, in *nota.Input, today time.Time) *InfDeclaracaoPrestacaoServico {
	if today.IsZero() {
		today = time.Now()
	}
	day := today.Format("2006-01-02")
	numero := cfg.Configuracoes.ProximoNumeroRPS

	aliquota := in.Servico.Aliquota
	if aliquota == 0 {
		aliquota = cfg.Configuracoes.AliquotaISS
	}

	cnae := in.Servico.CodigoCnae
	if cnae == "" {
		cnae = "0000000"
	}

	issRetidoFlag := ISSRetidoNao
	if in.Servico.IssRetido {
		issRetidoFlag = ISSRetidoSim
	}

	// Replicates the TS quirk: when issRetido is true, valorIss is reported as
	// the calculated retention; otherwise it is reported as 0.
	calcIss := in.Servico.ValorServicos * aliquota / 100
	var valorIss Dec2
	if in.Servico.IssRetido {
		valorIss = Dec2(0)
	} else {
		valorIss = Dec2(calcIss)
	}

	aliq := Dec1(aliquota)
	informacoes := in.Observacoes
	if informacoes == "" {
		informacoes = "Informação Complementar Solicitada"
	}

	servico := Servico{
		Valores: Valores{
			ValorServicos:          Dec2(in.Servico.ValorServicos),
			ValorDeducoes:          0,
			ValorPis:               0,
			ValorCofins:            0,
			ValorInss:              0,
			ValorIr:                0,
			ValorCsll:              0,
			OutrasRetencoes:        0,
			ValTotTributos:         0,
			ValorIss:               &valorIss,
			Aliquota:               &aliq,
			DescontoIncondicionado: 0,
			DescontoCondicionado:   0,
		},
		IssRetido:           issRetidoFlag,
		ResponsavelRetencao: RegimeEspecialPadrao, // TS hardcodes "1".
		ItemListaServico:    in.Servico.ItemListaServico,
		CodigoCnae:          cnae,
		Discriminacao:       in.Servico.Discriminacao,
		CodigoMunicipio:     cfg.Configuracoes.CodigoMunicipio,
		CodigoPais:          "1058", // TS hardcodes Brazil.
		ExigibilidadeISS:    ExigibilidadeExigivel,
		MunicipioIncidencia: cfg.Configuracoes.CodigoMunicipio,
	}

	prestador := Prestador{
		CpfCnpj:            CpfCnpj{CNPJ: cfg.Prestador.CNPJ},
		InscricaoMunicipal: cfg.Prestador.InscricaoMunicipal,
	}

	tomador := buildTomador(in)

	return &InfDeclaracaoPrestacaoServico{
		ID: "rps" + strconv.Itoa(numero),
		Rps: RpsHead{
			IdentificacaoRps: IdentificacaoRps{
				Numero: numero,
				Serie:  "00000", // TS hardcodes "00000" regardless of config.serieRps.
				Tipo:   TipoRPS,
			},
			DataEmissao: day,
			Status:      StatusRPSNormal,
		},
		Competencia:               day,
		Servico:                   servico,
		Prestador:                 prestador,
		TomadorServico:            tomador,
		RegimeEspecialTributacao:  RegimeEspecialPadrao,
		OptanteSimplesNacional:    OptanteSimplesNao,
		IncentivoFiscal:           IncentivadorCultNao,
		InformacoesComplementares: informacoes,
	}
}

func buildTomador(in *nota.Input) *TomadorServico {
	if in == nil {
		return nil
	}
	t := in.Tomador

	var ident *IdentificacaoTomador
	switch {
	case t.CNPJ != "":
		ident = &IdentificacaoTomador{CpfCnpj: CpfCnpj{CNPJ: t.CNPJ}}
	case t.CPF != "":
		ident = &IdentificacaoTomador{CpfCnpj: CpfCnpj{CPF: t.CPF}}
	}

	end := &Endereco{
		Endereco:        t.Endereco.Endereco,
		Numero:          t.Endereco.Numero,
		Complemento:     t.Endereco.Complemento,
		Bairro:          t.Endereco.Bairro,
		CodigoMunicipio: t.Endereco.CodigoMunicipio,
		UF:              t.Endereco.UF,
		CEP:             t.Endereco.CEP,
	}

	var contato *Contato
	if t.Telefone != "" || t.Email != "" {
		contato = &Contato{Telefone: t.Telefone, Email: t.Email}
	}

	return &TomadorServico{
		IdentificacaoTomador: ident,
		RazaoSocial:          t.RazaoSocial,
		Endereco:             end,
		Contato:              contato,
	}
}
