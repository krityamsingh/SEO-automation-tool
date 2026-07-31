package option

type ClientOption interface {
	clientOption()
}

type withAPIKey string

func (w withAPIKey) clientOption() {}

func WithAPIKey(key string) ClientOption {
	return withAPIKey(key)
}
