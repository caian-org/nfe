package abrasf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/caian-org/nfe/internal/abrasf"
)

func TestParseResponseWithErrors(t *testing.T) {
	body := []byte(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd">
  <ListaMensagemRetorno>
    <MensagemRetorno>
      <Codigo>E001</Codigo>
      <Mensagem>CNPJ inválido</Mensagem>
      <Correcao>Verifique o CNPJ do prestador</Correcao>
    </MensagemRetorno>
  </ListaMensagemRetorno>
</GerarNfseResposta>`)

	resp, err := abrasf.ParseResponse(body)
	require.NoError(t, err)
	require.True(t, resp.HasErrors())
	require.Len(t, resp.Mensagens, 1)
	assert.Equal(t, "E001", resp.Mensagens[0].Codigo)
	assert.Equal(t, "CNPJ inválido", resp.Mensagens[0].Mensagem)
	assert.Equal(t, "Verifique o CNPJ do prestador", resp.Mensagens[0].Correcao)
}

func TestParseResponseUnwrapsOutputXMLEntityEncoded(t *testing.T) {
	// Many ABRASF WS implementations wrap the inner XML in an <outputXML>
	// element whose content is HTML-entity-encoded.
	body := []byte(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd"><outputXML>&lt;ListaMensagemRetorno&gt;&lt;MensagemRetorno&gt;&lt;Codigo&gt;E042&lt;/Codigo&gt;&lt;Mensagem&gt;Erro&lt;/Mensagem&gt;&lt;/MensagemRetorno&gt;&lt;/ListaMensagemRetorno&gt;</outputXML></GerarNfseResposta>`)
	resp, err := abrasf.ParseResponse(body)
	require.NoError(t, err)
	require.True(t, resp.HasErrors())
	assert.Equal(t, "E042", resp.Mensagens[0].Codigo)
}

func TestParseResponseUnwrapsReturn(t *testing.T) {
	// JBoss-style wrappers use <return> instead of <outputXML>.
	body := []byte(`<resp><return>&lt;ListaMensagemRetorno&gt;&lt;MensagemRetorno&gt;&lt;Codigo&gt;X&lt;/Codigo&gt;&lt;Mensagem&gt;y&lt;/Mensagem&gt;&lt;/MensagemRetorno&gt;&lt;/ListaMensagemRetorno&gt;</return></resp>`)
	resp, err := abrasf.ParseResponse(body)
	require.NoError(t, err)
	assert.True(t, resp.HasErrors())
}

func TestParseResponseSuccess(t *testing.T) {
	body := []byte(`<GerarNfseResposta xmlns="http://www.abrasf.org.br/nfse.xsd">
  <ListaNfse>
    <CompNfse>
      <Nfse>
        <InfNfse>
          <Numero>42</Numero>
          <CodigoVerificacao>ABC123</CodigoVerificacao>
        </InfNfse>
      </Nfse>
    </CompNfse>
  </ListaNfse>
</GerarNfseResposta>`)

	resp, err := abrasf.ParseResponse(body)
	require.NoError(t, err)
	assert.False(t, resp.HasErrors())
	assert.Equal(t, "42", abrasf.FindNFSeNumero(resp.Raw))
}

func TestFindNFSeNumeroMissing(t *testing.T) {
	assert.Empty(t, abrasf.FindNFSeNumero([]byte(`<root/>`)))
}

func TestParseResponseEmpty(t *testing.T) {
	resp, err := abrasf.ParseResponse([]byte(``))
	require.NoError(t, err)
	assert.False(t, resp.HasErrors())
}

func TestParseResponseMultipleMessages(t *testing.T) {
	body := []byte(`<x><MensagemRetorno><Codigo>A</Codigo><Mensagem>a</Mensagem></MensagemRetorno><MensagemRetorno><Codigo>B</Codigo><Mensagem>b</Mensagem></MensagemRetorno></x>`)
	resp, err := abrasf.ParseResponse(body)
	require.NoError(t, err)
	require.Len(t, resp.Mensagens, 2)
	assert.Equal(t, "A", resp.Mensagens[0].Codigo)
	assert.Equal(t, "B", resp.Mensagens[1].Codigo)
}
