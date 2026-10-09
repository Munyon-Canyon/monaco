package errs

const CodePrivyUserLimit Code = "privy_user_limit"

func (codeFiles) PrivyUsers() map[Code]Row {
	return map[Code]Row{
		CodePrivyUserLimit: {
			Name: "PrivyUserLimit", Kind: KindUnavailable,
			Message: "The wallet provider cannot add more users. Try again later.",
		},
	}
}
