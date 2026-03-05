package adkruntime

type Config struct {
	Workspace string
}

type Runtime struct {
	cfg Config
}

func NewRuntime(cfg Config) *Runtime {
	return &Runtime{cfg: cfg}
}

func (r *Runtime) Config() Config {
	return r.cfg
}
