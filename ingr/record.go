package ingr

type Record interface {
	GetID() string
	GetValue(name string) any
	GetIntValue(name string) int
	GetStrValue(name string) string
	GetBoolValue(name string) bool
	GetComment(name string) string
	GetIsCommentedOut() bool

	SetIsCommentedOut() bool
	SetValue(name string, v any) error
	SetComment(name, value string) error
}
