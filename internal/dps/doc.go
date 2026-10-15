// Package dps builds the Declaração de Prestação de Serviço (DPS) XML of the
// Sistema Nacional NFS-e (layout v1.01, namespace
// http://www.sped.fazenda.gov.br/nfse). The SIGCORP "WS Padrão Nacional"
// used by Franco da Rocha receives this document unchanged.
//
// Struct field order mirrors the XSD sequence order and is therefore
// load-bearing: never reorder fields without regenerating the goldens.
package dps
