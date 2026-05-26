package abrasf

import (
	"encoding/xml"
	"fmt"
)

// BuildGerarNfse marshals a fully populated InfDeclaracaoPrestacaoServico
// into the GerarNfseEnvio envelope expected by the ABRASF web service. The
// returned bytes are NOT yet signed; pass them through xmlsig.Signer when a
// certificate is configured.
func BuildGerarNfse(inf *InfDeclaracaoPrestacaoServico) ([]byte, error) {
	if inf == nil {
		return nil, fmt.Errorf("BuildGerarNfse: declaração nula")
	}
	env := GerarNfseEnvio{
		Xmlns: Namespace,
		Rps: RpsContainer{
			Inf: *inf,
		},
	}
	out, err := xml.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("serializar GerarNfseEnvio: %w", err)
	}
	return out, nil
}
