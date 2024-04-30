package config

import "github.com/namsral/flag"

type Config struct {
	Port      string
	LogFormat string

	PGUserName string
	PGPassword string
	PGHost     string
	PGPort     string
	PGDBName   string

	GoogleAiApiKey string
}

func (c *Config) Init() {
	flag.StringVar(&c.Port, "listen_port", "8080", "The port for the server to listen on")
	flag.StringVar(&c.LogFormat, "log_format", "text", "The format of the logs. Either text, or json")

	flag.StringVar(&c.PGUserName, "pg_username", "", "PG DB username")
	flag.StringVar(&c.PGPassword, "pg_password", "", "PG DB password")
	flag.StringVar(&c.PGHost, "pg_hostname", "localhost", "PG DB hostname")
	flag.StringVar(&c.PGPort, "pg_port", "5432", "PG DB port")
	flag.StringVar(&c.PGDBName, "pg_dbname", "", "PG DB name")

	flag.StringVar(&c.GoogleAiApiKey, "googleai_api_key", "", "GoogleAI API key")

	flag.Parse()
}
