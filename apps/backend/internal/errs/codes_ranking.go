package errs

const (
	CodeConservationBroken Code = "conservation_broken"
)

func (codeFiles) Ranking() map[Code]Row {
	return map[Code]Row{
		CodeConservationBroken: {
			Name:    "ConservationBroken",
			Kind:    KindInternal,
			Alert:   true,
			Message: "Something went wrong.",
		},
	}
}
