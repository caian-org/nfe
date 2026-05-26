package abrasf

import (
	"encoding/xml"
	"fmt"
)

// Cancellation reason codes (CodigoCancelamento) accepted by ABRASF.
const (
	CancelErroEmissao        = 1
	CancelServicoNaoPrestado = 2
	CancelErroProcessamento  = 3
	CancelDuplicidade        = 4
)

// CancelInput bundles the parameters of a cancellation request.
type CancelInput struct {
	NumeroNfse         string
	CNPJ               string
	InscricaoMunicipal string
	CodigoMunicipio    string
	Codigo             int
}

// BuildCancelarNfse serializes a CancelarNfseEnvio. The returned bytes are
// NOT yet signed; xmlsig.Signer must wrap InfPedidoCancelamento when a
// certificate is configured.
func BuildCancelarNfse(in CancelInput) ([]byte, error) {
	if in.NumeroNfse == "" {
		return nil, fmt.Errorf("BuildCancelarNfse: número da NFS-e é obrigatório")
	}
	if in.Codigo < CancelErroEmissao || in.Codigo > CancelDuplicidade {
		return nil, fmt.Errorf("BuildCancelarNfse: codigo deve estar entre %d e %d, recebido %d",
			CancelErroEmissao, CancelDuplicidade, in.Codigo)
	}
	if in.CNPJ == "" || in.InscricaoMunicipal == "" || in.CodigoMunicipio == "" {
		return nil, fmt.Errorf("BuildCancelarNfse: CNPJ, inscricaoMunicipal e codigoMunicipio são obrigatórios")
	}

	env := CancelarNfseEnvio{
		Xmlns: Namespace,
		Pedido: PedidoContainer{
			Inf: InfPedidoCancelamento{
				IdentificacaoNfse: IdentificacaoNfse{
					Numero:             in.NumeroNfse,
					CpfCnpj:            CpfCnpj{CNPJ: in.CNPJ},
					InscricaoMunicipal: in.InscricaoMunicipal,
					CodigoMunicipio:    in.CodigoMunicipio,
				},
				CodigoCancelamento: in.Codigo,
			},
		},
	}

	out, err := xml.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("serializar CancelarNfseEnvio: %w", err)
	}
	return out, nil
}
