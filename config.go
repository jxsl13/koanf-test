package main

type RootConfig struct {
	ClientId     string `koanf:"client.id" description:"client id"`
	ClientSecret string `koanf:"client.secret" description:"client secret"`

	// optional (should be pointers)
	ConfigPath *string `koanf:"config" description:"Config file path (.env format)"`
}
