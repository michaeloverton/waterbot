package env

import (
	"github.com/kelseyhightower/envconfig"
)

// Environment is the configuration object that is populated at launch.
type Environment struct {
	Env           string `default:"local"`
	Port          string `default:"3000"`
	LogLevel      string `split_words:"true" default:"INFO"`
	BotScreenName string `split_words:"true" required:"true"`
	EspBaseURL    string `split_words:"true" required:"true" envconfig:"esp_base_url"`
	Twitter       struct {
		ApiKey            string `split_words:"true" required:"true"`
		ApiSecret         string `split_words:"true" required:"true"`
		AccessToken       string `split_words:"true" required:"true"`
		AccessTokenSecret string `split_words:"true" required:"true"`
	} `split_words:"true"`
}

// LoadEnv loads the configuration object from .env
func LoadEnv() (*Environment, error) {
	var e Environment
	err := envconfig.Process("", &e)
	if err != nil {
		return nil, err
	}
	return &e, nil
}
