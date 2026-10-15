package config

// Default returns the seed configuration written by `nfe init`. Prestador
// values are obvious placeholders; [nacional] points at the SIGCORP WS Padrão
// Nacional hosts and [ibscbs] carries the 2026 test-year rates.
func Default() *Config {
	ibsUF, ibsMun, cbs := 0.1, 0.0, 0.9
	return &Config{
		Ambiente: EnvHomologacao,
		SOAP: SOAP{
			WSDLHomologacao:  "https://teste.exemplo.com.br/abrasf/ws/nfs?wsdl",
			WSDLProducao:     "https://producao.exemplo.com.br/abrasf/ws/nfs?wsdl",
			SOAPActionPrefix: DefaultSOAPActionPrefix,
		},
		Prestador: Prestador{
			CNPJ:               "00000000000000",
			InscricaoMunicipal: "000000",
			RazaoSocial:        "EMPRESA EXEMPLO LTDA",
			NomeFantasia:       "EXEMPLO",
			Endereco: Endereco{
				Endereco:        "RUA EXEMPLO",
				Numero:          "100",
				Complemento:     "",
				Bairro:          "CENTRO",
				CodigoMunicipio: "0000000",
				UF:              "SP",
				CEP:             "00000000",
			},
			Contato: &Contato{
				Telefone: "1100000000",
				Email:    "contato@exemplo.com.br",
			},
		},
		Autenticacao: Autenticacao{
			Usuario: "usuario",
			Senha:   "senha",
		},
		Configuracoes: Configuracoes{
			SerieRPS:          "A",
			ProximoNumeroRPS:  1,
			CodigoMunicipio:   "0000000",
			AliquotaISS:       5.0,
			ConfirmTimeout:    DefaultConfirmTimeout,
			ConfirmInterval:   DefaultConfirmInterval,
			ProximoNumeroNFSe: 1,
		},
		Nacional: &Nacional{
			URLProducao:     DefaultNacionalURLProducao,
			URLHomologacao:  DefaultNacionalURLHomologacao,
			SerieDPS:        DefaultSerieDPS,
			VersaoAplicacao: DefaultVersaoAplicacao,
			OpSimpNac:       OpSimpNacNaoOptante,
			RegEspTrib:      0,
		},
		IBSCBS: &IBSCBS{
			CST:                           "000",
			CodigoClassificacaoTributaria: "000001",
			CodigoIndicadorOperacao:       "100301",
			AliquotaIBSUF:                 &ibsUF,
			AliquotaIBSMunicipal:          &ibsMun,
			AliquotaCBS:                   &cbs,
		},
	}
}
