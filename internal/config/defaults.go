package config

// Default returns the seed configuration written by `nfe init`. All values are
// obvious placeholders — the user is expected to edit them before pointing
// the CLI at a real ABRASF web service.
func Default() *Config {
	return &Config{
		Ambiente: EnvHomologacao,
		SOAP: SOAP{
			WSDLHomologacao: "https://teste.exemplo.com.br/abrasf/ws/nfs?wsdl",
			WSDLProducao:    "https://producao.exemplo.com.br/abrasf/ws/nfs?wsdl",
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
			SerieRPS:         "A",
			ProximoNumeroRPS: 1,
			CodigoMunicipio:  "0000000",
			AliquotaISS:      5.0,
			ConfirmTimeout:   DefaultConfirmTimeout,
			ConfirmInterval:  DefaultConfirmInterval,
		},
	}
}
