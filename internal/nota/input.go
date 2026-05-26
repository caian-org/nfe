// Package nota holds the TOML-shaped invoice input type consumed by `nfe emit`.
package nota

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Input is the user-facing TOML input parsed by `nfe emit`. It carries only
// the data that varies per invoice; per-prestador and per-environment fields
// live in config.Config.
type Input struct {
	Tomador     Tomador `toml:"tomador"`
	Servico     Servico `toml:"servico"`
	CNBS        string  `toml:"cnbs,omitempty"`
	Observacoes string  `toml:"observacoes,omitempty"`
}

type Tomador struct {
	CNPJ        string   `toml:"cnpj,omitempty"`
	CPF         string   `toml:"cpf,omitempty"`
	RazaoSocial string   `toml:"razao_social"`
	Email       string   `toml:"email,omitempty"`
	Telefone    string   `toml:"telefone,omitempty"`
	Endereco    Endereco `toml:"endereco"`
}

type Endereco struct {
	Endereco        string `toml:"endereco"`
	Numero          string `toml:"numero"`
	Complemento     string `toml:"complemento,omitempty"`
	Bairro          string `toml:"bairro"`
	CodigoMunicipio string `toml:"codigo_municipio"`
	UF              string `toml:"uf"`
	CEP             string `toml:"cep"`
}

type Servico struct {
	Discriminacao    string  `toml:"discriminacao"`
	ValorServicos    float64 `toml:"valor_servicos"`
	ItemListaServico string  `toml:"item_lista_servico"`
	CodigoCnae       string  `toml:"codigo_cnae,omitempty"`
	IssRetido        bool    `toml:"iss_retido,omitempty"`
	Aliquota         float64 `toml:"aliquota,omitempty"`
}

var cNBSRegex = regexp.MustCompile(`^\d{9}$`)

// Load reads a TOML invoice input from path and validates the required fields.
func Load(path string) (*Input, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("arquivo de entrada não encontrado: %s", path)
		}
		return nil, fmt.Errorf("leitura do input: %w", err)
	}

	var in Input
	if err := toml.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse do input: %w", err)
	}

	if errs := Validate(&in); len(errs) > 0 {
		return nil, fmt.Errorf("input inválido: %s", strings.Join(errs, "; "))
	}
	return &in, nil
}

// Validate returns user-friendly validation errors for in, mirroring the TS
// validateNotaFiscal function.
func Validate(in *Input) []string {
	var errs []string

	if in.Tomador.RazaoSocial == "" {
		errs = append(errs, "razão social do tomador é obrigatória")
	}
	if in.Tomador.CNPJ == "" && in.Tomador.CPF == "" {
		errs = append(errs, "CNPJ ou CPF do tomador é obrigatório")
	}
	if in.Servico.Discriminacao == "" {
		errs = append(errs, "discriminação do serviço é obrigatória")
	}
	if in.Servico.ValorServicos <= 0 {
		errs = append(errs, "valor dos serviços deve ser maior que zero")
	}
	if in.Servico.ItemListaServico == "" {
		errs = append(errs, "item da lista de serviços é obrigatório")
	}
	if in.CNBS != "" && !cNBSRegex.MatchString(in.CNBS) {
		errs = append(errs, "código NBS (cnbs) deve conter exatamente 9 dígitos numéricos")
	}

	return errs
}

// Example returns the example invoice written by `nfe init`. All values are
// placeholders — the user replaces them with real data before emitting.
func Example() *Input {
	return &Input{
		Tomador: Tomador{
			CNPJ:        "00000000000000",
			RazaoSocial: "TOMADOR EXEMPLO LTDA",
			Email:       "contato@tomador-exemplo.com.br",
			Telefone:    "1100000000",
			Endereco: Endereco{
				Endereco:        "RUA EXEMPLO",
				Numero:          "100",
				Complemento:     "",
				Bairro:          "CENTRO",
				CodigoMunicipio: "0000000",
				UF:              "SP",
				CEP:             "00000000",
			},
		},
		Servico: Servico{
			Discriminacao:    "DESCRIÇÃO DO SERVIÇO PRESTADO",
			ValorServicos:    1000.0,
			ItemListaServico: "0101",
			CodigoCnae:       "6201500",
			IssRetido:        false,
			Aliquota:         5.0,
		},
		CNBS:        "115022000",
		Observacoes: "Observações sobre o serviço prestado",
	}
}
