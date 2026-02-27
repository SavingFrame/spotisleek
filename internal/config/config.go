package config

import (
	"errors"
	"log/slog"
	"reflect"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	NAVIDROME_URL      string `mapstructure:"NAVIDROME_URL"`
	NAVIDROME_USERNAME string `mapstructure:"NAVIDROME_USERNAME"`
	NAVIDROME_PASSWORD string `mapstructure:"NAVIDROME_PASSWORD"`
	SLSKD_URL          string `mapstructure:"SLSKD_URL"`
	SLSKD_API_KEY      string `mapstructure:"SLSKD_API_KEY"`
}

func LoadConfig() (config Config, err error) {
	viper.AddConfigPath(".")
	viper.SetConfigName(".env")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	if err := bindEnvForStruct[Config](); err != nil {
		return config, err
	}

	var fileLookupError viper.ConfigFileNotFoundError
	if err := viper.ReadInConfig(); err != nil {
		if errors.As(err, &fileLookupError) {
			slog.Warn("No config file found, relying on environment variables")
		} else {
			return config, err
		}
	}

	err = viper.Unmarshal(&config)
	return
}

func bindEnvForStruct[T any]() error {
	t := reflect.TypeOf((*T)(nil)).Elem()

	for i := range t.NumField() {
		field := t.Field(i)
		key := field.Tag.Get("mapstructure")
		if key == "" || key == "-" {
			continue
		}

		key = strings.Split(key, ",")[0]
		if key == "" {
			continue
		}

		if err := viper.BindEnv(key); err != nil {
			return err
		}
	}

	return nil
}
