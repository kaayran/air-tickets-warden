package api

import "encoding/json"

// Optional distinguishes the three JSON states a PATCH body field can be in:
// key absent (Set=false — leave the stored value untouched), key explicitly
// null (Set=true, Value=nil — clear back to the cascade default), and key with
// a value (Set=true, Value!=nil). encoding/json never calls UnmarshalJSON for
// absent keys, which is what makes the zero value mean "absent".
type Optional[T any] struct {
	Set   bool
	Value *T
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}
