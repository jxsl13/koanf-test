package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/providers/structs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type parseOption struct {
	envPrefix      string
	delimiter      string
	tag            string
	flatStruct     bool
	helpFlag       string
	configPathKey  string
	descriptionTag string
}

type ParseOption func(*parseOption)

func WithEnvPrefix(prefix string) ParseOption {
	return func(po *parseOption) {
		po.envPrefix = prefix
	}
}

func WithDelimiter(delimiter string) ParseOption {
	return func(po *parseOption) {
		po.delimiter = delimiter
	}
}

func WithStructTagName(tag string) ParseOption {
	return func(po *parseOption) {
		po.tag = tag
	}
}

func WithDescriptionStructTagName(tag string) ParseOption {
	return func(po *parseOption) {
		po.descriptionTag = tag
	}
}

type Validatable interface {
	Validate() error
}

func RegisterFlags[T any](config *T, persistent bool, app *cobra.Command, options ...ParseOption) (parse func() error) {

	op := parseOption{
		envPrefix:      "SNKD_",
		delimiter:      ".",
		tag:            "koanf",
		flatStruct:     true,
		helpFlag:       "help",
		configPathKey:  "config",
		descriptionTag: "description",
	}

	for _, o := range options {
		o(&op)
	}

	transform := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(s, "_", op.delimiter))
	}

	f := func(s string) string {
		return transform(strings.TrimPrefix(s, op.envPrefix))
	}

	k := koanf.New(op.delimiter)

	// does not error
	_ = k.Load(structs.ProviderWithDelim(config, op.tag, op.delimiter), nil)
	knownKeys := append(keys(k.All()), op.configPathKey)

	var fs *pflag.FlagSet
	if persistent {
		fs = app.PersistentFlags()
	} else {
		fs = app.Flags()
	}

	// disable unknonwn flags errors
	before := fs.ParseErrorsWhitelist
	fs.ParseErrorsWhitelist.UnknownFlags = true
	defer func() {
		fs.ParseErrorsWhitelist = before
	}()

	fs.StringP(op.configPathKey, "c", "", fmt.Sprintf(".env config file path (or env variable %s%s)", op.envPrefix, strings.ToUpper(op.configPathKey)))

	ct := reflect.TypeOf(config)
	if ct.Kind() == reflect.Pointer {
		ct = ct.Elem()
	}

	for key, v := range filter(k.All(), knownKeys) {
		desc := description(op.tag, key, op.descriptionTag, ct)

		// key is now a flag name
		flagName := strings.ReplaceAll(key, op.delimiter, "-")

		switch x := v.(type) {
		case bool:
			fs.Bool(flagName, x, desc)
		default:
			if flagName == op.configPathKey {
				// already defined manually above
				continue
			}

			strValue := ""
			if v != nil {
				strValue = fmt.Sprintf("%v", v)
			}
			fs.String(flagName, strValue, desc)
		}
	}

	return func() error {
		err := k.Load(env.Provider(op.envPrefix, op.delimiter, f), nil)
		if err != nil {
			return err
		}

		err = fs.Parse(os.Args)
		if err != nil {
			return fmt.Errorf("failed to parse config flags: %w", err)
		}

		flagK := koanf.New(op.delimiter)
		err = flagK.Load(
			posflag.ProviderWithValue(
				fs,
				op.delimiter,
				nil,
				func(key, value string) (string, interface{}) {
					return transform(key), value
				},
			), nil)
		if err != nil {
			return err
		}

		if flagK.Bool(op.helpFlag) {
			return nil
		}

		// flags found -> use flags
		// flags not found -> env found -> use env
		for _, k := range []*koanf.Koanf{flagK, k} {
			configPath := k.String(op.configPathKey)
			if configPath == "" {
				continue
			}

			err = k.Load(file.Provider(configPath), dotenv.ParserEnv(op.envPrefix, op.delimiter, f))
			if err != nil {
				return err
			}
			break
		}

		// merge flag map into struct map
		_ = k.Load(confmap.Provider(flagK.All(), "-"), nil)

		err = k.UnmarshalWithConf("", config, koanf.UnmarshalConf{
			FlatPaths: op.flatStruct,
		})
		if err != nil {
			return err
		}

		var a any = config
		if v, ok := a.(Validatable); ok {
			return v.Validate()
		}

		return nil
	}
}

func keys(m map[string]any) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

func filter(m map[string]any, keys []string) map[string]any {
	lookup := make(map[string]bool, len(keys))
	for _, k := range keys {
		lookup[k] = true
	}
	result := make(map[string]any, len(keys))
	for k, v := range m {
		if lookup[k] {
			result[k] = v
		}
	}

	return result
}

func Marshal(cfgs []any, options ...ParseOption) ([]byte, error) {
	op := parseOption{
		envPrefix:      "SNKD_",
		delimiter:      ".",
		tag:            "koanf",
		flatStruct:     true,
		helpFlag:       "help",
		configPathKey:  "config",
		descriptionTag: "description",
	}

	for _, o := range options {
		o(&op)
	}

	k := koanf.New(op.delimiter)

	for _, cfg := range cfgs {
		// does not error
		_ = k.Load(structs.ProviderWithDelim(cfg, op.tag, op.delimiter), nil)
	}

	transform := func(s string) string {
		return op.envPrefix + strings.ToUpper(strings.ReplaceAll(s, op.delimiter, "_"))
	}

	m, _ := maps.Flatten(k.All(), nil, op.delimiter)

	envMap := make(map[string]any, len(m))
	for k, v := range m {
		envMap[transform(k)] = v
	}

	k = koanf.New(op.delimiter)

	// load transformed map (with correct keys)
	// and then marshal it with those keys
	_ = k.Load(confmap.Provider(envMap, ""), nil)

	return k.Marshal(dotenv.Parser())
}

func description(tag, tagValue, descriptionTag string, ct reflect.Type) string {
	numFields := ct.NumField()

	for i := 0; i < numFields; i++ {
		field := ct.Field(i)

		value := field.Tag.Get(tag)
		if value == tagValue {
			return field.Tag.Get(descriptionTag)
		}
	}

	return ""
}
