package ingr

import "fmt"

func NewMapRecordEntry[TKey comparable](id TKey, data map[string]any) Record {
	return mapRecordEntry[TKey]{
		id:   id,
		data: data,
	}
}

type mapRecordEntry[TKey comparable] struct {
	id   TKey
	data map[string]any
}

func (r mapRecordEntry[TKey]) GetID() string {
	return fmt.Sprintf("%v", r.id)
}

func (r mapRecordEntry[TKey]) GetData() map[string]any {
	return r.data
}

func (r mapRecordEntry[TKey]) GetValue(name string) any {
	//TODO implement me
	panic("implement me")
}

func (r mapRecordEntry[TKey]) GetIntValue(name string) int {
	//TODO implement me
	panic("implement me")
}

func (r mapRecordEntry[TKey]) GetStrValue(name string) string {
	//TODO implement me
	panic("implement me")
}

func (r mapRecordEntry[TKey]) GetBoolValue(name string) bool {
	//TODO implement me
	panic("implement me")
}

func (r mapRecordEntry[TKey]) IsCommented() bool {
	//TODO implement me
	panic("implement me")
}
