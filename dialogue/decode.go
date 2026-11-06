package dialogue

import (
	"encoding/json"
	"fmt"
	"io"
)

type decoder struct{ dec *json.Decoder }

func newDecoder(r io.Reader) decoder {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return decoder{dec}
}

// decode reads one JSON value into v and rejects anything after it.
func (d decoder) decode(v any) error {
	if err := d.dec.Decode(v); err != nil {
		return err
	}
	if d.dec.More() {
		return fmt.Errorf("data after the JSON value")
	}
	return nil
}
