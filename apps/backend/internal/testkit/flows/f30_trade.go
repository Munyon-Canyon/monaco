package flows

func (defined) StepsF30FundCabal() FlowStep {
	return FlowStep{Flow: "30", Order: 2, Name: "Fund cabal", Owner: "#4970", Status: StepPending}
}

func (defined) StepsF30RoutedBuy() FlowStep {
	return FlowStep{Flow: "30", Order: 3, Name: "Routed buy on EVM", Owner: "#4970", Status: StepPending}
}
