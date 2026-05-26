// Package abrasf models the ABRASF v2.04 XML wire formats consumed and
// produced by the NFS-e web service, and the builders that translate
// configuration plus invoice input into ready-to-sign envelopes.
//
// Element order is significant: the XSD requires the children of each
// element to appear in a specific sequence, so the struct fields below are
// declared in that sequence. Reordering fields will produce XML that the WS
// rejects.
package abrasf

// Namespace is the ABRASF schema namespace, applied to every envelope's root
// element via an explicit xmlns attribute (rather than Go's native namespace
// machinery, which renders prefixed declarations that the WS rejects).
const Namespace = "http://www.abrasf.org.br/nfse.xsd"
