package resolver

import (
	"os"
	"os/user"
	"regexp"
	"strings"
)

var dynamicPattern = regexp.MustCompile(`\$\{DYN_([\w\d]+)(\.([\w\d]+))?\}`)

type dynamic struct{}

func NewDynamic() Resolver {
	return dynamic{}
}

func (dynamic) Init() error {
	return nil
}

func (dynamic) Name() string {
	return "dynamic"
}

func (d dynamic) Find(key string) (any, bool) {
	name, function, found := d.parse(key)
	if !found {
		return nil, false
	}

	value, found := dynamicValue(name)
	if !found || function == "" {
		return value, found
	}
	return applyDynamicFunction(function, value)
}

func (dynamic) parse(key string) (name, function string, found bool) {
	matches := dynamicPattern.FindStringSubmatch(key)
	if len(matches) == 0 {
		return "", "", false
	}
	return matches[1], matches[3], true
}

func dynamicValue(name string) (any, bool) {
	switch strings.ToUpper(name) {
	case "LOCAL_HOST":
		host, err := os.Hostname()
		return host, err == nil
	case "USER_NAME":
		current, err := user.Current()
		if err != nil {
			return nil, false
		}
		return current.Name, true
	case "WORKSPACE_ROOT":
		path, err := os.Getwd()
		return path, err == nil
	default:
		return nil, false
	}
}

func applyDynamicFunction(function string, value any) (any, bool) {
	switch strings.ToLower(function) {
	case "delspace":
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		return strings.Join(strings.Fields(text), ""), true
	default:
		return nil, false
	}
}
