package resolver

import "os"

type environment struct{}

func NewEnvironment() Resolver {
	return environment{}
}

func (environment) Init() error {
	return nil
}

func (environment) Name() string {
	return "environment"
}

func (environment) Find(key string) (any, bool) {
	if len(key) >= 3 && key[:2] == "${" && key[len(key)-1:] == "}" {
		key = key[2 : len(key)-1]
	}
	value, found := os.LookupEnv(key)
	return value, found
}
