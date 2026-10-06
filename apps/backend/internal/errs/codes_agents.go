package errs

const (
	CodeAgentNotFound         Code = "agent_not_found"
	CodeAgentExists           Code = "agent_exists"
	CodeAgentWrongStatus      Code = "agent_wrong_status"
	CodeAgentPaused           Code = "agent_paused"
	CodeAgentBudgetExceeded   Code = "agent_budget_exceeded"
	CodeAgentHoldingsExceeded Code = "agent_holdings_exceeded"
)

func (codeFiles) Agents() map[Code]Row {
	return map[Code]Row{
		CodeAgentNotFound: {Name: "AgentNotFound", Kind: KindNotFound, Message: "This cabal has no agent."},
		CodeAgentExists: {
			Name: "AgentExists", Kind: KindConflict,
			Message: "This cabal already has an agent. Remove it before adding another.",
		},
		CodeAgentWrongStatus: {
			Name: "AgentWrongStatus", Kind: KindConflict,
			Message: "The agent can't make that change from its current status.",
		},
		CodeAgentPaused: {
			Name: "AgentPaused", Kind: KindForbidden,
			Message: "This agent is paused. The cabal can vote to resume it.",
		},
		CodeAgentBudgetExceeded: {
			Name: "AgentBudgetExceeded", Kind: KindBlocked,
			Message: "This trade is over the agent's remaining budget.",
		},
		CodeAgentHoldingsExceeded: {
			Name: "AgentHoldingsExceeded", Kind: KindBlocked,
			Message: "The agent can only sell what it bought and the cabal still holds.",
		},
	}
}
