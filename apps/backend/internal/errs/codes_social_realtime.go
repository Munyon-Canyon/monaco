package errs

const CodeNoRealtimeChannels Code = "no_realtime_channels"

func (codeFiles) SocialRealtime() map[Code]Row {
	return map[Code]Row{
		CodeNoRealtimeChannels: {
			Name: "NoRealtimeChannels", Kind: KindConflict, Message: "Join a cabal to get live chat updates.",
		},
	}
}
