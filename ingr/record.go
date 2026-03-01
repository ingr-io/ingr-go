package ingr

type Record interface {
	GetID() string
	GetValue(name string) any
	GetIntValue(name string) int
	GetStrValue(name string) string
	GetBoolValue(name string) bool
	IsCommented() bool
}
