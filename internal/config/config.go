package config

import "github.com/caarlos0/env/v11"

type Config struct {
	WorkDir       string   `env:"WORKDIR" envDefault:"/app/workdir"`
	OutputDir     string   `env:"OUTPUT_DIR" envDefault:"/app/public"`
	RepoURL       string   `env:"REPO_URL,required,notEmpty"`
	RepoBranch    string   `env:"REPO_BRANCH" envDefault:"main"`
	WebhookSecret string   `env:"WEBHOOK_SECRET"`
	ServerAddress string   `env:"SERVER_ADDRESS" envDefault:":8080"`
	PollInterval  int      `env:"POLL_INTERVAL" envDefault:"0"` // in seconds
	HugoArgs      []string `env:"HUGO_ARGS" envSeparator:"," envDefault:"--minify,--gc"`
}

func New() (*Config, error) {
	var cfg Config
	err := env.Parse(&cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
