package conf

var (
	// use the address of Config
	globalCfg = &Config{}
)

func GetGlobalConfig() *Config {
	return globalCfg
}

func SetGlobalConfig(c *Config) {
	globalCfg = c
}
