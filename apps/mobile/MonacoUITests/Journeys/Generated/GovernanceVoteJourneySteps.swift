enum GovernanceVoteJourneySteps {
    static let id = "governance/vote"
    static let version = 7

    static let s1p1 = JourneyStep(
        id: "S1.1",
        name: "tap the Home tab",
        actor: "A",
        seconds: 15,
        copy: ["Your vote", "0"]
    )
    static let s1p2 = JourneyStep(
        id: "S1.2",
        name: "tap the card's row (home-vote-open-<proposalID>)",
        actor: "A",
        seconds: 15,
        copy: [
            "Price",
            "Shares",
            "Of treasury",
            "Closes",
            "QA vote {QA.run}",
            "Yes, 0 votes",
            "No, 0 votes",
            "Remind me at close",
        ]
    )
    static let s1p3 = JourneyStep(
        id: "S1.3",
        name: "tap proposal-vote-yes, and let go before 0.6 s",
        actor: "A",
        seconds: 15,
        copy: ["Yes, 0 votes", "No, 0 votes"]
    )
    static let s1p4 = JourneyStep(
        id: "S1.4",
        name: "tap proposal-vote-yes",
        actor: "A",
        seconds: 10,
        copy: ["Voted Yes, 1 vote", "1 of 3 voted", "Voted Yes. 1st of 3.", "Undo", "Hear how votes end?", "Not now"]
    )
    static let s1p5 = JourneyStep(
        id: "S1.5",
        name: "tap proposal-change-vote Change, then proposal-vote-no, then",
        actor: "A",
        seconds: 10,
        copy: [
            "You voted Yes",
            "Yes, 0 votes",
            "No, 1 vote",
            "Changed to No.",
            "Change",
            "Yes, 1 vote",
            "No, 0 votes",
            "Changed to Yes.",
        ]
    )
    static let s1p6 = JourneyStep(
        id: "S1.6",
        name: "tap the Home tab, then home-vote-open-<proposalID>",
        actor: "B",
        seconds: 15,
        copy: ["Yes, 1 vote", "No, 0 votes", "1 of 3 voted"]
    )
    static let s1p7 = JourneyStep(
        id: "S1.7",
        name: "pick and hold proposal-vote-no, then the toast's Undo while",
        actor: "B",
        seconds: 10,
        copy: ["Voted No, 1 vote", "Voted No. 2nd of 3.", "Undo", "Yes, 1 vote", "No, 0 votes", "Vote undone."]
    )
    static let s1p8 = JourneyStep(
        id: "S1.8",
        name: "pick and hold proposal-vote-yes",
        actor: "B",
        seconds: 10,
        copy: ["Voted Yes. Passed 2–0.", "Voted Yes, 2 votes", "Passed 2–0", "Passed"]
    )
    static let s1p9 = JourneyStep(
        id: "S1.9",
        name: "relaunch the Home tab",
        actor: "A",
        seconds: 15,
        copy: ["Your vote"]
    )
    static let s2p1 = JourneyStep(
        id: "S2.1",
        name: "open the cabal the Cabals tab, then the cabal named",
        actor: "A",
        seconds: 15,
        copy: []
    )
    static let s2p2 = JourneyStep(
        id: "S2.2",
        name: "tap cabal-open-proposal-open-<proposalID>, the card's header",
        actor: "A",
        seconds: 15,
        copy: []
    )
    static let s2p3 = JourneyStep(
        id: "S2.3",
        name: "pick and hold proposal-vote-yes",
        actor: "A",
        seconds: 10,
        copy: ["Voted Yes.", "Undo"]
    )
    static let s3p1 = JourneyStep(
        id: "S3.1",
        name: "tap the Home tab, then All proposals in the Your vote header",
        actor: "A",
        seconds: 15,
        copy: ["All proposals"]
    )
    static let s3p2 = JourneyStep(
        id: "S3.2",
        name: "tap the Closed segment (proposals-segment-closed)",
        actor: "A",
        seconds: 15,
        copy: ["Expired", "Failed"]
    )
    static let s4p1 = JourneyStep(
        id: "S4.1",
        name: "open the rule-on cabal's closed proposal from All proposals,",
        actor: "A",
        seconds: 15,
        copy: ["✓ 2", "1 ✕", "you"]
    )
    static let s4p2 = JourneyStep(
        id: "S4.2",
        name: "open the rule-off cabal's closed proposal",
        actor: "A",
        seconds: 15,
        copy: []
    )
}
