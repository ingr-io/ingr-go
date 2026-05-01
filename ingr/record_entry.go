package ingr

import "fmt"

// NewMapRecordEntry creates a Record backed by a map.
// TKey is the type of the record's $ID (e.g. string, int).
func NewMapRecordEntry[TKey comparable](id TKey, data map[string]any) Record {
	return &mapRecordEntry[TKey]{
		id:       id,
		data:     data,
		comments: make(map[string]string),
	}
}

type mapRecordEntry[TKey comparable] struct {
	id             TKey
	isCommentedOut bool
	data           map[string]any
	comments       map[string]string
}

func (r *mapRecordEntry[TKey]) GetIsCommentedOut() bool { return r.isCommentedOut }

func (r *mapRecordEntry[TKey]) SetIsCommentedOut() bool {
	r.isCommentedOut = true
	return r.isCommentedOut
}

func (r *mapRecordEntry[TKey]) SetValue(name string, v any) error {
	r.data[name] = v
	return nil
}

func (r *mapRecordEntry[TKey]) SetComment(name, value string) error {
	r.comments[name] = value
	return nil
}

func (r *mapRecordEntry[TKey]) GetComment(name string) string { return r.comments[name] }

func (r *mapRecordEntry[TKey]) GetID() string { return fmt.Sprintf("%v", r.id) }

func (r *mapRecordEntry[TKey]) GetValue(name string) any {
	if name == "$ID" {
		return r.id
	}
	return r.data[name]
}

func (r *mapRecordEntry[TKey]) GetIntValue(name string) int   { return r.data[name].(int) }
func (r *mapRecordEntry[TKey]) GetStrValue(name string) string { return r.data[name].(string) }
func (r *mapRecordEntry[TKey]) GetBoolValue(name string) bool  { return r.data[name].(bool) }
