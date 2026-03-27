package ingr

// RawValue is a raw encoded INGR/JSON value.
// It implements Marshaler and Unmarshaler and can be used to delay
// JSON decoding or to store pre-computed JSON output.
type RawValue []byte

// MarshalINGR returns r as the INGR encoding of r.
func (r RawValue) MarshalINGR() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return r, nil
}

// UnmarshalINGR sets *r to a copy of data.
func (r *RawValue) UnmarshalINGR(data []byte) error {
	if r == nil {
		return &SyntaxError{msg: "ingr.RawValue: UnmarshalINGR on nil pointer"}
	}
	*r = append((*r)[:0], data...)
	return nil
}
