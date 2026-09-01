package resolver

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type file struct {
	path       string
	configType string
	values     *viper.Viper
}

func NewFile(path, configType string) Resolver {
	return &file{path: path, configType: configType}
}

func (f *file) Init() error {
	f.values = viper.New()
	info, err := os.Stat(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("placeholder source %q is a directory", f.path)
	}

	f.values.SetConfigFile(f.path)
	f.values.SetConfigType(f.configType)
	if err := f.values.ReadInConfig(); err != nil {
		return err
	}
	f.values.SetTypeByDefaultValue(false)
	return nil
}

func (f *file) Name() string {
	return f.path
}

func (f *file) Find(key string) (any, bool) {
	if len(key) >= 3 && key[:2] == "${" && key[len(key)-1:] == "}" {
		key = key[2 : len(key)-1]
	}
	if f.values == nil || !f.values.IsSet(key) {
		return nil, false
	}
	return f.values.Get(key), true
}
