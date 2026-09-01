package resolver

type Resolver interface {
	Init() error
	Name() string
	Find(key string) (any, bool)
}
