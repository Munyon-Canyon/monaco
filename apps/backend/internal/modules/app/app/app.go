package app

type Config struct {
	MinIOSBuild int32
	StoreURL    string
}

type GetConfig struct {
	cfg Config
}

func NewGetConfig(cfg Config) GetConfig { return GetConfig{cfg: cfg} }

func (q GetConfig) Handle() Config { return q.cfg }
