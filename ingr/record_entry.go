package ingr

import "fmt"

func NewMapRecordEntry[TKey comparable](id TKey, data map[string]any) Record {
	return mapRecordEntry[TKey]{
		id:   id,
		data: data,
	}
}

type mapRecordEntry[TKey comparable] struct {
	id TKey

	// We want to preserve the record but don't want it to get parsed.
	isCommentedOut bool

	// holds values of fields of a record
	data map[string]any

	// holds comments for fields of a record
	comments map[string]string
}

func (r mapRecordEntry[TKey]) GetIsCommentedOut() bool {
	return r.isCommentedOut
}

func (r mapRecordEntry[TKey]) SetIsCommentedOut() bool {
	//TODO implement me
	panic("implement me")
}

func (r mapRecordEntry[TKey]) SetValue(name string, v any) error {
	r.data[name] = v
	return nil
}

func (r mapRecordEntry[TKey]) SetComment(name, value string) error {
	r.comments[name] = value
	return nil
}

func (r mapRecordEntry[TKey]) GetComment(name string) string {
	return r.comments[name]
}

func (r mapRecordEntry[TKey]) GetID() string {
	return fmt.Sprintf("%v", r.id)
}

func (r mapRecordEntry[TKey]) GetData() map[string]any {
	return r.data
}

func (r mapRecordEntry[TKey]) GetValue(name string) any {
	return r.data[name]
}

func (r mapRecordEntry[TKey]) GetIntValue(name string) int {
	return r.data[name].(int)
}

func (r mapRecordEntry[TKey]) GetStrValue(name string) string {
	return r.data[name].(string)
}

func (r mapRecordEntry[TKey]) GetBoolValue(name string) bool {
	return r.data[name].(bool)
}

func (r mapRecordEntry[TKey]) IsCommentedOut() bool {
	return r.isCommentedOut
}
