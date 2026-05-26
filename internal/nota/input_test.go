package nota_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/nota"
)

func TestLoadMinimal(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "nota_minimal.toml")
	in, err := nota.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "44555666000170", in.Tomador.CNPJ)
	assert.Equal(t, "DESCRIÇÃO DO SERVIÇO PRESTADO", in.Servico.Discriminacao)
	assert.Equal(t, 15000.0, in.Servico.ValorServicos)
	assert.False(t, in.Servico.IssRetido)
	assert.Equal(t, 2.0, in.Servico.Aliquota)
}

func TestLoadMissing(t *testing.T) {
	_, err := nota.Load("/no/such/path.toml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "não encontrado")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		fix  func(*nota.Input)
		want string
	}{
		{
			name: "missing razao social",
			fix:  func(in *nota.Input) { in.Tomador.RazaoSocial = "" },
			want: "razão social do tomador",
		},
		{
			name: "no cnpj nor cpf",
			fix: func(in *nota.Input) {
				in.Tomador.CNPJ = ""
				in.Tomador.CPF = ""
			},
			want: "CNPJ ou CPF do tomador",
		},
		{
			name: "missing discriminacao",
			fix:  func(in *nota.Input) { in.Servico.Discriminacao = "" },
			want: "discriminação do serviço",
		},
		{
			name: "zero valor servicos",
			fix:  func(in *nota.Input) { in.Servico.ValorServicos = 0 },
			want: "valor dos serviços",
		},
		{
			name: "missing item lista",
			fix:  func(in *nota.Input) { in.Servico.ItemListaServico = "" },
			want: "item da lista",
		},
		{
			name: "cNBS with non-numeric",
			fix:  func(in *nota.Input) { in.CNBS = "12345678a" },
			want: "9 dígitos numéricos",
		},
		{
			name: "cNBS too short",
			fix:  func(in *nota.Input) { in.CNBS = "1234" },
			want: "9 dígitos numéricos",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := nota.Example()
			tc.fix(in)
			errs := nota.Validate(in)
			require.NotEmpty(t, errs)
			joined := ""
			for _, e := range errs {
				joined += e + "\n"
			}
			assert.Contains(t, joined, tc.want)
		})
	}
}

func TestExamplePassesValidation(t *testing.T) {
	errs := nota.Validate(nota.Example())
	require.Empty(t, errs)
}

func TestCPFOnlyTomadorPasses(t *testing.T) {
	in := nota.Example()
	in.Tomador.CNPJ = ""
	in.Tomador.CPF = "12345678909"
	errs := nota.Validate(in)
	require.Empty(t, errs)
}
