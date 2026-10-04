import Foundation

/// User-facing strings for the proposal feed, detail thread, and composer.
/// Kept here so host tests can audit them against `MainFlowCopyAudit`.
public enum ProposalFeedCopy {
    public static let feedTitle = "Proposals"
    public static let feedLinkTitle = "All proposals"
    public static let emptyOpen = "No open votes. Propose a buy to get the cabal voting."
    public static let emptyClosed = "No closed votes yet."
    public static let loadFailed = "Couldn't load proposals. Pull down to try again"

    public static let needsYourVote = "Needs your vote"
    public static let voteYes = "Yes"
    public static let voteNo = "No"
    public static let voteRecorded = "Vote in"
    public static let voteClosed = "Voting on this has closed"
    public static let voteNotEligible = "You can't vote on this one"
    public static let voteFailed = "Couldn't send your vote. Try again"

    public static let commentsTitle = "Discussion"
    public static let emptyThread = "No comments yet. Make the case for or against this buy."
    public static let composerPlaceholder = "Add a comment"
    public static let reply = "Reply"
    public static let send = "Post"
    public static let commentPosted = "Comment posted"
    public static let replyPosted = "Reply posted"
    public static let commentsLoadFailed = "Couldn't load comments"
    public static let commentTooLong = "Comments can be up to 1,000 characters"
    public static let commentRejected = "That comment couldn't be posted. Edit it and try again"
    public static let commentRateLimited = "You're commenting fast. Try again in a minute"
    public static let commentUnavailable = "This proposal is gone"
    public static let commentFailed = "Couldn't post that. Try again"

    public static func replyingTo(_ name: String) -> String {
        "Replying to \(name)"
    }

    public static func commentCount(_ count: Int) -> String {
        count == 1 ? "1 comment" : "\(count) comments"
    }

    public static func proposedBy(_ name: String) -> String {
        "Proposed by \(name)"
    }

    public static func buyHeadline(symbol: String, amount: String) -> String {
        "Buy \(amount) of \(symbol)"
    }

    public static func sellHeadline(symbol: String, shares: String) -> String {
        "Sell \(shares) \(symbol)"
    }

    public static let agentTitle = "Cabal agent"

    public static func viewerVoted(_ choice: String) -> String {
        choice.lowercased() == "no" ? "You voted no" : "You voted yes"
    }

    // MARK: Detail

    public static let votesTitle = "Votes"
    public static let ballotYes = "Yes"
    public static let ballotNo = "No"
    public static let ballotWaiting = "Waiting"
    public static let statusTitle = "Status"
    public static let detailLoadFailed = "Couldn't load this proposal. Pull down to try again"
    public static let tryAgain = "Try again"
    public static let replyAccessibility = "Reply"
    public static let postAccessibility = "Post comment"

    /// Tracker step labels: "Voting", "Buying" / "Selling", "Done".
    public static func trackerSteps(isSell: Bool) -> [String] {
        ["Voting", isSell ? "Selling" : "Buying", "Done"]
    }

    public static func executionFailed(isSell: Bool) -> String {
        isSell ? "The sell didn't go through. Retry from Activity." : "The buy didn't go through. Retry from Activity."
    }

    public static func executionPending(isSell: Bool) -> String {
        isSell ? "Selling now. This takes about a minute." : "Buying now. This takes about a minute."
    }

    public static func openCount(_ count: Int) -> String {
        count == 1 ? "1 open vote" : "\(count) open votes"
    }

    /// Every static string plus representative formatted ones, for copy audits.
    public static let auditedStrings: [String] =
        [
            feedTitle, feedLinkTitle, emptyOpen, emptyClosed, loadFailed,
            needsYourVote, voteYes, voteNo, voteRecorded, voteClosed, voteNotEligible, voteFailed,
            commentsTitle, emptyThread, composerPlaceholder, reply, send,
            commentPosted, replyPosted, commentsLoadFailed, commentTooLong,
            commentRejected, commentRateLimited, commentUnavailable, commentFailed,
            replyingTo("Ada"), commentCount(2), proposedBy("Ada"),
            buyHeadline(symbol: "AAPLx", amount: "$25.00"), sellHeadline(symbol: "AAPLx", shares: "0.5"), openCount(3),
            agentTitle,
            viewerVoted("yes"), viewerVoted("no"),
            "Didn't pass", "Expired", "Failed", "Bought", "Sold", "Buying", "Selling", "Passed",
            votesTitle, ballotYes, ballotNo, ballotWaiting, statusTitle, detailLoadFailed, tryAgain,
            replyAccessibility, postAccessibility,
            executionFailed(isSell: false), executionFailed(isSell: true),
            executionPending(isSell: false), executionPending(isSell: true),
        ] + trackerSteps(isSell: false) + trackerSteps(isSell: true)
}

/// Strings for the propose chooser and the Buy / Sell / trading bot flows.
public enum ProposeFlowCopy {
    public static let chooserTitle = "Propose"
    public static let buyRow = "Buy a stock"
    public static let buyRowDetail = "Your cabal votes on it first"
    public static let sellRow = "Sell something the cabal owns"
    public static let sellRowEmpty = "Nothing to sell yet"
    public static let addBotRow = "Add a trading bot"
    public static let addBotRowDetail = "Give a bot a budget from the pot"
    public static let pauseBotRow = "Pause the trading bot"
    public static let resumeBotRow = "Turn the trading bot back on"
    public static let removeBotRow = "Remove the trading bot"

    // Step 1
    public static let buyTitle = "Buy"
    public static let searchPlaceholder = "Search Apple, Tesla, NVDA…"
    public static let popularTitle = "Popular"
    public static let resultsTitle = "Stocks"
    public static let cantBuy = "Can't buy right now"
    public static let stocksLoadFailed = "Couldn't load stocks. Try again"
    public static func noMatches(_ query: String) -> String {
        "No stocks match “\(query)”"
    }

    // Step 2
    public static let amountTitle = "Amount"
    public static func potHelper(_ amount: String) -> String {
        "The pot has \(amount)"
    }
    public static let overPot = "More than the pot has"
    public static let addReason = "Add a reason"
    public static let reasonPlaceholderBuy = "Why should the cabal buy this?"
    public static let reasonPlaceholderSell = "Why should the cabal sell this?"
    public static let review = "Review"
    public static let potLoadFailed = "Couldn't load the pot. Try again"
    /// Longest reason (proposal thesis) the backend accepts, measured by `reasonLength`.
    public static let reasonMax = 500
    /// The counter appears only near the limit.
    public static let reasonCounterFrom = 400
    /// Length the backend checks: the trimmed thesis in UTF-8 bytes, so the limit here never
    /// passes text the server would reject. Equals the character count for plain text.
    public static func reasonLength(_ text: String) -> Int {
        text.trimmingCharacters(in: .whitespacesAndNewlines).utf8.count
    }
    public static func reasonCounter(_ count: Int) -> String {
        "\(count) of \(reasonMax)"
    }
    public static let reasonTooLong = "Keep the reason under 500 characters"

    // Step 3
    public static let youreProposing = "You're proposing"
    public static let youreSelling = "You're proposing to sell"
    public static func ofStock(_ name: String) -> String {
        "of \(name)"
    }
    public static let priceRow = "Price"
    public static func perShare(_ price: String) -> String {
        "\(price) a share"
    }
    public static let sharesRow = "Shares"
    public static func aboutShares(_ shares: String) -> String {
        "About \(shares)"
    }
    public static let cabalRow = "Cabal"
    public static let reasonRow = "Reason"
    public static let sendToCabal = "Send to cabal"
    public static func proposalSent(_ cabalName: String) -> String {
        "Proposal sent to \(cabalName)"
    }
    public static let proposalSentGeneric = "Proposal sent to your cabal"

    // Errors
    public static let priceCheckFailed = "Couldn't check the price. Try again"
    public static func cantBuyStock(_ name: String) -> String {
        "Can't buy \(name) right now. Try a smaller amount or another stock."
    }
    public static let changeAmount = "Change amount"
    public static let sendFailed = "Couldn't send this proposal. Try again"
    public static let noConnection = "No connection. Check your internet and try again"

    // Sell
    public static let sellTitle = "Sell"
    public static let holdingsTitle = "What the cabal owns"
    public static let sellTooSmall = "Can't sell that little. Try a bigger amount."
    public static let sellNoLongerAvailable = "The cabal doesn't hold that much anymore"
    public static func sellSummary(amount: String, name: String, shares: String) -> String {
        "Sell about \(amount) of \(name) (\(shares))"
    }
    public static func sellHelper(_ amount: String) -> String {
        "The cabal holds \(amount)"
    }
    public static let overHoldings = "More than the cabal holds"

    // Trading bot
    public static let addBotTitle = "Add a trading bot"
    public static let botNamePlaceholder = "Bot name"
    public static let botBudgetHelper = "Budget from the pot"
    public static let botExplainer = "Your cabal votes first. Once it passes, you'll get a key to paste into your bot."
    public static let botKeyExplainer =
        "Paste this key into your bot. It stays here for 15 minutes. After that, anyone in the cabal can copy it from the bot\u{2019}s screen."
    public static let agentKeyExplainer =
        "Paste this key into your bot. Anyone in the cabal can copy it here until the bot is removed."
    public static let agentDetailTitle = "Trading bot"
    public static let agentKeySection = "Bot key"
    public static let agentKeyMissing =
        "No key on file. If this bot was added before keys were saved, remove it and add a new bot."
    public static let copyKey = "Copy key"
    /// The header over the key on the proposal screen; the button under it says "Copy key".
    public static let botKeyTitle = "Bot key"
    public static let keyCopied = "Key copied"
    public static let copyConnectInstructions = "Copy connect instructions"
    public static let connectCopied = "Connect instructions copied"
    public static let clawPumpSteps =
        "In ClawPump, paste these into your agent as a custom skill. Then add an automation that runs it every hour."
    public static func lifecycleTitle(kind: String) -> String {
        switch kind {
        case "pause_agent": "Pause the trading bot?"
        case "resume_agent": "Turn the trading bot back on?"
        default: "Remove the trading bot?"
        }
    }
    public static func lifecycleMessage(kind: String, botName: String) -> String {
        switch kind {
        case "pause_agent": "Your cabal votes on it. \(botName) stops trading once it passes."
        case "resume_agent": "Your cabal votes on it. \(botName) starts trading again once it passes."
        default: "Your cabal votes on it. Once it passes, \(botName) can't trade again and its key stops working."
        }
    }

    public static let auditedStrings: [String] = [
        chooserTitle, buyRow, buyRowDetail, sellRow, sellRowEmpty, addBotRow, addBotRowDetail,
        pauseBotRow, resumeBotRow, removeBotRow,
        buyTitle, searchPlaceholder, popularTitle, resultsTitle, cantBuy, stocksLoadFailed, noMatches("Apple"),
        amountTitle, potHelper("$548.20"), overPot, addReason, reasonPlaceholderBuy, reasonPlaceholderSell,
        review, potLoadFailed, reasonCounter(480), reasonTooLong,
        youreProposing, youreSelling, ofStock("Apple"), priceRow, perShare("$231.40"), sharesRow,
        aboutShares("0.2161 shares"), cabalRow, reasonRow, sendToCabal, proposalSent("Weekend investors"),
        proposalSentGeneric, priceCheckFailed, cantBuyStock("Apple"), changeAmount, sendFailed, noConnection,
        sellTitle, holdingsTitle, sellTooSmall, sellNoLongerAvailable,
        sellSummary(amount: "$139", name: "Apple", shares: "0.6 shares"), sellHelper("$278.47"), overHoldings,
        addBotTitle, botNamePlaceholder, botBudgetHelper, botExplainer, botKeyExplainer, agentKeyExplainer,
        agentDetailTitle, agentKeySection, agentKeyMissing, copyKey, botKeyTitle, keyCopied,
        copyConnectInstructions, connectCopied, clawPumpSteps,
        lifecycleTitle(kind: "pause_agent"), lifecycleTitle(kind: "resume_agent"), lifecycleTitle(kind: "revoke_agent"),
        lifecycleMessage(kind: "pause_agent", botName: "Scout"),
        lifecycleMessage(kind: "resume_agent", botName: "Scout"),
        lifecycleMessage(kind: "revoke_agent", botName: "Scout"),
    ]
}
