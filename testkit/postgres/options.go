package postgres

type options struct {
	image    string
	database string
}

type Option func(*options)

func WithImage(image string) Option {
	return func(o *options) { o.image = image }
}

func WithDatabase(database string) Option {
	return func(o *options) { o.database = database }
}
