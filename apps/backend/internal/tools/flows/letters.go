package flows

func CheckLetterConsumers(flows []Flow, env Env, ids []string) []Problem {
	var problems []Problem
	if env.UnstatedLetters == nil {
		return problems
	}
	for _, f := range flows {
		if !selected(ids, f.ID) || !f.Status.AtLeastBuilt() {
			continue
		}
		for _, command := range f.Commands {
			for _, o := range f.Outcomes {
				name := ScriptName(f, command, o)
				if n := env.UnstatedLetters(name); n > 0 {
					problems = append(problems, f.problemf(
						"script %s declares %d dead letters without a consumer; set Consumer on each Letter in "+
							"internal/testkit/flows so monacoctl verify compares which consumer wrote it",
						name, n,
					))
				}
			}
		}
	}
	return problems
}
