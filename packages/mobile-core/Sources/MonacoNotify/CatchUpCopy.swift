import Foundation
import MonacoAPI

public enum CatchUpCopy {
    public typealias Item = Components.Schemas.CatchUpItem

    public static let title = "Catch up"
    public static let oneByOne = "One by one"
    public static let loadingLabel = "Loading Catch up"

    public static let waitingHeader = "Waiting on you"
    public static let repliesHeader = "Replies and mentions"
    public static let invitesHeader = "Invites"
    public static let resultsHeader = "Results"
    public static let clearedHeader = "Cleared today"

    public static let caughtUpTitle = "You're caught up"
    public static let openFeed = "Open Feed"
    public static let clearAction = "Clear"
    public static let clearFailedToast = "Couldn't clear that. Try again."
    public static let firstUseLine = "When a cabal you're in votes, it lands here first."

    public static func subLine(count: Int) -> String {
        "\(count) to catch up · what you can still change comes first"
    }

    public static func joinedToast(_ cabalName: String) -> String {
        "You're in \(cabalName)."
    }

    public static func symbol(_ proposal: Components.Schemas.CatchUpProposal) -> String {
        let symbol = proposal.symbol
        guard symbol.count > 1, symbol.hasSuffix("x") else { return symbol }
        return String(symbol.dropLast())
    }

    public static func proposalTitle(_ proposal: Components.Schemas.CatchUpProposal) -> String {
        switch proposal.kind {
        case "buy": "Buy \(symbol(proposal))"
        case "sell": "Sell \(symbol(proposal))"
        default: "Rule change"
        }
    }

    public static func dollars(micros: Int64) -> String {
        let whole = micros / 1_000_000
        let cents = (micros % 1_000_000) / 10_000
        let grouped = groupDigits(whole)
        return cents == 0 ? "$\(grouped)" : "$\(grouped).\(cents < 10 ? "0" : "")\(cents)"
    }

    private static func groupDigits(_ value: Int64) -> String {
        let digits = String(value)
        var out = ""
        for (index, char) in digits.reversed().enumerated() {
            if index > 0, index % 3 == 0 { out.append(",") }
            out.append(char)
        }
        return String(out.reversed())
    }

    public static func waitingTitle(_ proposal: Components.Schemas.CatchUpProposal) -> String {
        let title = proposalTitle(proposal)
        guard proposal.kind == "buy", let micros = proposal.usdcMicros else { return title }
        return "\(title) · \(dollars(micros: micros))"
    }

    public static func waitingSubtitle(_ item: Item, now: Date) -> String {
        guard let proposal = item.proposal else { return item.cabal?.name ?? "" }
        let ends = endsIn(proposal.expiresAt, now: now)
        return [item.cabal?.name, "\(proposal.voted) of \(proposal.eligible) voted", ends]
            .compactMap { $0 }.joined(separator: " · ")
    }

    public static func tally(_ proposal: Components.Schemas.CatchUpProposal) -> (yes: String, no: String) {
        ("\(proposal.yes)", "\(proposal.no)")
    }

    public static func endsIn(_ expires: Date, now: Date) -> String {
        let seconds = expires.timeIntervalSince(now)
        guard seconds > 60 else { return "ends now" }
        return "ends in \(span(seconds))"
    }

    private static func span(_ seconds: TimeInterval) -> String {
        if seconds < 3600 { return "\(Int(seconds / 60))m" }
        if seconds < 86400 { return "\(Int(seconds / 3600))h" }
        return "\(Int(seconds / 86400))d"
    }

    public static func personName(_ item: Item) -> String {
        let name = item.actor?.name ?? ""
        return name.isEmpty ? "Someone" : name
    }

    public static func rowTitle(_ item: Item) -> String {
        switch item.kind {
        case .mention: "\(personName(item)) mentioned you"
        case .reply: "\(personName(item)) replied to you"
        case .invite: "\(personName(item)) invited you to \(item.cabal?.name ?? "a cabal")"
        case .waitingVote: item.proposal.map(waitingTitle) ?? ""
        case .result: item.proposal.map(resultTitle) ?? ""
        }
    }

    public static func replyAbout(_ item: Item) -> String? {
        if let proposal = item.proposal { return proposalTitle(proposal) }
        guard let post = item.post else { return nil }
        return post.actorName.isEmpty ? post.label : "\(post.actorName)'s \(post.label)"
    }

    public static func replySubtitle(_ item: Item) -> String {
        [item.cabal?.name, replyAbout(item)].compactMap { $0 }.joined(separator: " · ")
    }

    public static func quote(_ item: Item) -> (author: String, text: String)? {
        guard let quote = item.quote else { return nil }
        return (quote.isYou ? "You" : quote.authorName, quote.text)
    }

    public static func inviteSubtitle(_ item: Item) -> String {
        guard let invite = item.invite else { return "" }
        let members = invite.memberCount == 1 ? "1 member" : "\(invite.memberCount) members"
        guard let holding = invite.topHoldingName else { return members }
        return "\(members) · holds \(holding)"
    }

    public static func resultTitle(_ proposal: Components.Schemas.CatchUpProposal) -> String {
        let verb: String
        switch proposal.outcome {
        case .filled, .fillFailed: verb = "passed"
        case .failed: verb = "failed"
        case .expired, nil: verb = "closed"
        }
        return "\(proposalTitle(proposal)) \(verb) \(proposal.yes)–\(proposal.no)"
    }

    public static func resultSubtitle(_ item: Item) -> String {
        guard let proposal = item.proposal else { return item.cabal?.name ?? "" }
        var parts: [String] = [item.cabal?.name].compactMap { $0 }
        switch proposal.yourChoice {
        case .yes: parts.append("you voted Yes")
        case .no: parts.append("you voted No")
        case nil: break
        }
        if proposal.outcome == .filled, proposal.filled == true { parts.append("filled") }
        return parts.joined(separator: " · ")
    }

    public static func subtitle(_ item: Item, now: Date) -> String {
        switch item.kind {
        case .waitingVote: waitingSubtitle(item, now: now)
        case .reply, .mention: replySubtitle(item)
        case .invite: inviteSubtitle(item)
        case .result: resultSubtitle(item)
        }
    }

    public static func relativeTime(_ at: Date, now: Date, calendar: Calendar) -> String {
        let seconds = now.timeIntervalSince(at)
        if seconds < 60 { return "now" }
        if seconds < 3600 { return "\(Int(seconds / 60))m" }
        if seconds < 86400 { return "\(Int(seconds / 3600))h" }
        if seconds < 7 * 86400 { return "\(Int(seconds / 86400))d" }
        let parts = calendar.dateComponents([.month, .day], from: at)
        let month = calendar.shortMonthSymbols[(parts.month ?? 1) - 1]
        return "\(month) \(parts.day ?? 1)"
    }

    public static func caughtUpLine(cabals: [Components.Schemas.CatchUpCabal]) -> String {
        let names: String
        switch cabals.count {
        case 0: return firstUseLine
        case 1: names = cabals[0].name
        case 2: names = "\(cabals[0].name) or \(cabals[1].name)"
        case 3: names = "\(cabals[0].name), \(cabals[1].name) or \(cabals[2].name)"
        default: names = "your cabals"
        }
        return "Nothing in \(names) waits on you. The next vote lands here first."
    }

    public static func clearedSubtitle(_ item: Item, now: Date) -> String {
        guard let reason = item.clearedReason else { return subtitle(item, now: now) }
        let cabal = item.cabal?.name
        switch reason {
        case .voted:
            if item.kind == .result { return resultSubtitle(item) }
            let choice = item.proposal?.yourChoice.map { $0 == .yes ? "you voted Yes" : "you voted No" } ?? "you voted"
            return [cabal, choice].compactMap { $0 }.joined(separator: " · ")
        case .replied:
            return [cabal, "you replied"].compactMap { $0 }.joined(separator: " · ")
        case .joined:
            return [cabal, "you joined"].compactMap { $0 }.joined(separator: " · ")
        case .cleared:
            guard item.kind == .invite else { return subtitle(item, now: now) }
            return [inviteSubtitle(item), "later"].filter { !$0.isEmpty }.joined(separator: " · ")
        }
    }

    public static func spokenTime(_ at: Date, now: Date, calendar: Calendar) -> String {
        let seconds = now.timeIntervalSince(at)
        if seconds < 60 { return "just now" }
        if seconds < 3600 { return plural(Int(seconds / 60), "minute") + " ago" }
        if seconds < 86400 { return plural(Int(seconds / 3600), "hour") + " ago" }
        if seconds < 7 * 86400 { return plural(Int(seconds / 86400), "day") + " ago" }
        return relativeTime(at, now: now, calendar: calendar)
    }

    private static func plural(_ value: Int, _ unit: String) -> String {
        value == 1 ? "1 \(unit)" : "\(value) \(unit)s"
    }

    public static func accessibilityLabel(_ item: Item, now: Date, calendar: Calendar) -> String {
        var label: String
        switch item.kind {
        case .mention, .reply:
            label = "\(rowTitle(item))"
            if let cabal = item.cabal?.name { label += " in \(cabal)" }
            if let about = replyAbout(item) { label += ", \(about)" }
            if let quote = quote(item) {
                let spoken = quote.text.replacingOccurrences(of: ":", with: ",")
                    .replacingOccurrences(of: "@", with: "at ")
                label += ": \(spoken)"
            }
        case .waitingVote, .result, .invite:
            let line = item.clearedReason == nil ? subtitle(item, now: now) : clearedSubtitle(item, now: now)
            label = [rowTitle(item), line].filter { !$0.isEmpty }.joined(separator: ", ")
        }
        label += " \(spokenTime(item.at, now: now, calendar: calendar))"
        if item.unread { label += ", unread" }
        return label + "."
    }
}
