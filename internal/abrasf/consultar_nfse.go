package abrasf

import (
	"encoding/xml"
	"fmt"
)

// ConsultaQuery captures the user-facing filters of the `query` command.
// Either Numero or both DataInicial and DataFinal must be present.
type ConsultaQuery struct {
	Numero      string
	DataInicial string
	DataFinal   string
	Pagina      int // Defaults to 1 when zero.
}

// BuildConsultarServicoPrestado serializes a ConsultarNfseServicoPrestadoEnvio
// using the same envelope shape as the original TypeScript builder.
func BuildConsultarServicoPrestado(cnpj, inscricao string, q ConsultaQuery) ([]byte, error) {
	if cnpj == "" || inscricao == "" {
		return nil, fmt.Errorf("BuildConsultarServicoPrestado: CNPJ e inscrição municipal do prestador são obrigatórios")
	}
	if q.Numero == "" && (q.DataInicial == "" || q.DataFinal == "") {
		return nil, fmt.Errorf("BuildConsultarServicoPrestado: informe numero ou ambos data_inicial+data_final")
	}

	page := q.Pagina
	if page == 0 {
		page = 1
	}

	env := ConsultarNfseServicoPrestadoEnvio{
		Xmlns: Namespace,
		Prestador: Prestador{
			CpfCnpj:            CpfCnpj{CNPJ: cnpj},
			InscricaoMunicipal: inscricao,
		},
		NumeroNfse: q.Numero,
		Pagina:     page,
	}
	if q.DataInicial != "" && q.DataFinal != "" {
		env.PeriodoCompetencia = &PeriodoCompetencia{
			DataInicial: q.DataInicial,
			DataFinal:   q.DataFinal,
		}
	}

	out, err := xml.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("serializar ConsultarNfseServicoPrestadoEnvio: %w", err)
	}
	return out, nil
}

// BuildConsultarPorRPS serializes a ConsultarNfseRpsEnvio request for the RPS
// identifier used by GerarNfse.
func BuildConsultarPorRPS(cnpj, inscricao string, rps IdentificacaoRps) ([]byte, error) {
	if cnpj == "" || inscricao == "" {
		return nil, fmt.Errorf("BuildConsultarPorRPS: CNPJ e inscrição municipal do prestador são obrigatórios")
	}
	if rps.Numero == 0 || rps.Serie == "" || rps.Tipo == 0 {
		return nil, fmt.Errorf("BuildConsultarPorRPS: identificação do RPS incompleta")
	}

	env := ConsultarNfsePorRpsEnvio{
		Xmlns:            Namespace,
		IdentificacaoRps: rps,
		Prestador: Prestador{
			CpfCnpj:            CpfCnpj{CNPJ: cnpj},
			InscricaoMunicipal: inscricao,
		},
	}

	out, err := xml.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("serializar ConsultarNfseRpsEnvio: %w", err)
	}
	return out, nil
}
