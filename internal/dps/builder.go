package dps

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/caian-org/nfe/internal/config"
	"github.com/caian-org/nfe/internal/nota"
)

// Build maps the configured prestador and a per-invoice input into a DPS.
// now is injected so tests can pin dhEmi/dCompet; the zero value means time.Now.
func Build(cfg *config.Config, in *nota.Input, now time.Time) (*DPS, error) {
	if cfg == nil || in == nil {
		return nil, errors.New("dps: config e nota são obrigatórios")
	}
	if cfg.Nacional == nil {
		return nil, errors.New("dps: seção [nacional] ausente no config.toml")
	}
	if now.IsZero() {
		now = time.Now()
	}

	var errs []string
	if in.Servico.CodigoTributacaoNacional == "" {
		errs = append(errs, "servico.codigo_tributacao_nacional é obrigatório no layout nacional (ex.: \"010101\" para o item 01.01)")
	}
	if cfg.IBSCBS != nil && in.CNBS == "" {
		errs = append(errs, "cnbs é obrigatório quando [ibscbs] está configurado")
	}
	toma, err := buildTomador(in)
	if err != nil {
		errs = append(errs, err.Error())
	}
	prest, err := buildPrestador(cfg)
	if err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("dps: %s", strings.Join(errs, "; "))
	}

	nDPS := strconv.Itoa(cfg.Configuracoes.ProximoNumeroRPS)
	cLocEmi := cfg.Configuracoes.CodigoMunicipio
	serie := strings.TrimLeft(cfg.Nacional.SerieDPS, "0")
	if serie == "" {
		serie = "0"
	}

	locPrest := in.Servico.LocalPrestacao
	if locPrest == "" {
		locPrest = cLocEmi
	}

	tribMun := TribMun{TribISSQN: TribISSQNTributavel, TpRetISSQN: TpRetISSQNNaoRetido}
	if in.Servico.IssRetido {
		tribMun.TpRetISSQN = TpRetISSQNRetidoTomador
	}
	// The platform resolves the ISS rate from its own table except for Simples
	// Nacional prestadores and services rendered outside the issuing município.
	if cfg.Nacional.OpSimpNac != config.OpSimpNacNaoOptante || locPrest != cLocEmi {
		aliq := in.Servico.Aliquota
		if aliq == 0 {
			aliq = cfg.Configuracoes.AliquotaISS
		}
		p := Dec2(aliq)
		tribMun.PAliq = &p
	}

	serv := Serv{
		LocPrest: LocPrest{CLocPrestacao: locPrest},
		CServ: CServ{
			CTribNac:  in.Servico.CodigoTributacaoNacional,
			XDescServ: in.Servico.Discriminacao,
			CNBS:      in.CNBS,
		},
	}
	if in.Observacoes != "" {
		serv.InfoCompl = &InfoCompl{XInfComp: in.Observacoes}
	}

	d := &DPS{
		Xmlns:  Namespace,
		Versao: Versao,
		InfDPS: InfDPS{
			ID:       ID(cLocEmi, cfg.Prestador.CNPJ, serie, nDPS),
			TpAmb:    cfg.TpAmb(),
			DhEmi:    now.Format("2006-01-02T15:04:05-07:00"),
			VerAplic: cfg.VersaoAplicacao(),
			Serie:    serie,
			NDPS:     nDPS,
			DCompet:  now.Format("2006-01-02"),
			TpEmit:   TpEmitPrestador,
			CLocEmi:  cLocEmi,
			Prest:    prest,
			Toma:     toma,
			Serv:     serv,
			Valores: Valores{
				VServPrest: VServPrest{VServ: Dec2(in.Servico.ValorServicos)},
				Trib: Trib{
					TribMun: tribMun,
					TribFed: &TribFed{PisCofins: PisCofins{CST: CSTPisCofinsNenhum, TpRetPisCofins: TpRetPisCofinsNaoRetidos}},
					TotTrib: TotTrib{IndTotTrib: IndTotTribNaoInformado},
				},
			},
			IBSCBS: buildIBSCBS(cfg, in),
		},
	}
	return d, nil
}

// ID composes the 45-character infDPS identifier (Anexo I, campo 101).
func ID(cLocEmi, cnpj, serie, nDPS string) string {
	return "DPS" + cLocEmi + "2" + leftPad(digits(cnpj), 14) + leftPad(serie, 5) + leftPad(nDPS, 15)
}

// digits keeps only 0-9: the national layout types CNPJ, CPF, CEP and fone
// as numeric strings, while notas are often written with punctuation.
func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fone normalizes a phone number to TSTelefone ([0-9]{6,20}); anything
// shorter is dropped because the element is optional and a bad value is a
// structural rejection.
func fone(s string) string {
	d := digits(s)
	if len(d) < 6 || len(d) > 20 {
		return ""
	}
	return d
}

func leftPad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat("0", n-len(s)) + s
}

func buildPrestador(cfg *config.Config) (Prestador, error) {
	pr := cfg.Prestador
	e := pr.Endereco
	if pr.RazaoSocial == "" || e.CodigoMunicipio == "" || e.CEP == "" || e.Endereco == "" || e.Numero == "" || e.Bairro == "" {
		return Prestador{}, errors.New("prestador.razao_social e prestador.endereco (codigo_municipio, cep, endereco, numero, bairro) são obrigatórios: a plataforma rejeita a DPS sem nome e endereço do prestador")
	}
	p := Prestador{
		CNPJ:  digits(pr.CNPJ),
		IM:    pr.InscricaoMunicipal,
		XNome: pr.RazaoSocial,
		End: &Endereco{
			EndNac:  EnderecoNac{CMun: e.CodigoMunicipio, CEP: digits(e.CEP)},
			XLgr:    e.Endereco,
			Nro:     e.Numero,
			XCpl:    e.Complemento,
			XBairro: e.Bairro,
		},
		RegTrib: RegTrib{OpSimpNac: cfg.Nacional.OpSimpNac, RegEspTrib: cfg.Nacional.RegEspTrib},
	}
	if c := pr.Contato; c != nil {
		p.Fone = fone(c.Telefone)
		p.Email = strings.TrimSpace(c.Email)
	}
	return p, nil
}

func buildTomador(in *nota.Input) (*Pessoa, error) {
	t := in.Tomador
	p := &Pessoa{CNPJ: digits(t.CNPJ), CPF: digits(t.CPF), XNome: t.RazaoSocial, Fone: fone(t.Telefone), Email: strings.TrimSpace(t.Email)}
	e := t.Endereco
	filled := e.CodigoMunicipio != "" || e.CEP != "" || e.Endereco != "" || e.Numero != "" || e.Bairro != ""
	if !filled {
		return p, nil
	}
	if e.CodigoMunicipio == "" || e.CEP == "" || e.Endereco == "" || e.Numero == "" || e.Bairro == "" {
		return nil, errors.New("endereço do tomador incompleto: codigo_municipio, cep, endereco, numero e bairro são obrigatórios no layout nacional")
	}
	p.End = &Endereco{
		EndNac:  EnderecoNac{CMun: e.CodigoMunicipio, CEP: digits(e.CEP)},
		XLgr:    e.Endereco,
		Nro:     e.Numero,
		XCpl:    e.Complemento,
		XBairro: e.Bairro,
	}
	return p, nil
}

func buildIBSCBS(cfg *config.Config, in *nota.Input) *IBSCBS {
	if cfg.IBSCBS == nil {
		return nil
	}
	cst := cfg.IBSCBS.CST
	classTrib := cfg.IBSCBS.CodigoClassificacaoTributaria
	indOp := cfg.IBSCBS.CodigoIndicadorOperacao
	if o := in.IBSCBS; o != nil {
		if o.CST != "" {
			cst = o.CST
		}
		if o.CodigoClassificacaoTributaria != "" {
			classTrib = o.CodigoClassificacaoTributaria
		}
		if o.CodigoIndicadorOperacao != "" {
			indOp = o.CodigoIndicadorOperacao
		}
	}
	return &IBSCBS{
		FinNFSe: FinNFSeRegular,
		CIndOp:  indOp,
		IndDest: IndDestTomador,
		Valores: IBSCBSValores{Trib: IBSCBSTrib{GIBSCBS: GIBSCBS{CST: cst, CClassTrib: classTrib}}},
	}
}

// Marshal serializes the DPS with an XML declaration. The bytes are unsigned;
// pass them through xmlsig.Signer.SignSibling("infDPS", d.InfDPS.ID) when a
// signature is required.
func Marshal(d *DPS) ([]byte, error) {
	if d == nil {
		return nil, errors.New("dps: documento nulo")
	}
	body, err := xml.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("dps: serializar: %w", err)
	}
	return append([]byte(xml.Header), body...), nil
}
