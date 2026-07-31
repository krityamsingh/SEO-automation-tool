package option

type ClientOption interface {
	clientOption()
}

type withAPIKey string

func (w withAPIKey) clientOption() {}

func (w withAPIKey) String() string { return string(w) }

func WithAPIKey(key string) ClientOption {
	return withAPIKey(key)
}
