package events

func agentsRegistrations() []Registration {
	return []Registration{
		Register[AgentEnabled](TypeAgentEnabled, 1),
		Register[AgentPaused](TypeAgentPaused, 1),
		Register[AgentRemoved](TypeAgentRemoved, 1),
		Register[AgentChangeBlocked](TypeAgentChangeBlocked, 1),
		Register[AgentKeyRevealed](TypeAgentKeyRevealed, 1),
		Register[AgentIntentCreated](TypeAgentIntentCreated, 1),
	}
}
