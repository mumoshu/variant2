package variant

func RunMain(env Env, opts ...Option) error {
	cmd, path, args := GetPathAndArgsFromEnv(env)

	m, err := Load(FromPath(path, func(m *Main) {
		m.Command = cmd

		for _, o := range opts {
			o(m)
		}
	}))
	if err != nil {
		panic(err)
	}

	return m.Run(args, RunOptions{DisableLocking: false})
}
