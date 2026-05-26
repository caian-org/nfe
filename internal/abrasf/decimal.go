package abrasf

import (
	"encoding/xml"
	"strconv"
)

// Dec2 marshals a float as a decimal with exactly two fractional digits,
// matching the TypeScript Number.prototype.toFixed(2) output used for every
// monetary field in the original implementation.
type Dec2 float64

func (d Dec2) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(strconv.FormatFloat(float64(d), 'f', 2, 64), start)
}

// Dec1 marshals a float with exactly one fractional digit, matching the TS
// toFixed(1) used for the Aliquota field. ABRASF rejects 5.00 in places that
// expect 5.0, so the precision has to be exact.
type Dec1 float64

func (d Dec1) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(strconv.FormatFloat(float64(d), 'f', 1, 64), start)
}
