package errs

const CodePrivyUserLimit Code = "privy_user_limit"

func (codeFiles) PrivyUsers() map[Code]Row {
	return map[Code]Row{
		CodePrivyUserLimit: {
			Name: "PrivyUserLimit", Kind: KindUnavailable,
			Message: "We can't add more people right now. Try again later.",
		},
	}
}
