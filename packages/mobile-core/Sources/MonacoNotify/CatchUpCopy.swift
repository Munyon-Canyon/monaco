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
    public static let clearAction = "Clear"
    public static let caughtUpTitle = "You're caught up"
    public static let openFeed = "Open Feed"
    public static let clearFailedToast = "Couldn't clear that. Try again."
    public static let noCabalsLine = "When a cabal you're in votes, it lands here first."

    public static func caughtUpLine(cabals: [Components.Schemas.CatchUpCabal]) -> String {
        let names = cabals.map(\.name)
        let who: String
        switch names.count {
        case 0: return noCabalsLine
        case 1: who = names[0]
        case 2, 3: who = names.dropLast().joined(separator: ", ") + " or " + names[names.count - 1]
        default: who = "your cabals"
        }
        return "Nothing in \(who) waits on you. The next vote lands here first."
    }

    public static func clearedSubtitle(_ item: Item, now: Date) -> String {
        let cabal = item.kind == .invite ? nil : item.cabal?.name
        switch item.clearedReason {
        case .voted:
            if item.kind == .result { return resultSubtitle(item) }
            let vote = item.proposal?.yourChoice.map { "you voted \($0 == .yes ? "Yes" : "No")" } ?? "you voted"
            return [cabal, vote].compactMap { $0 }.joined(separator: " · ")
        case .replied: return [cabal, "you replied"].compactMap { $0 }.joined(separator: " · ")
        case .joined: return [cabal, "you joined"].compactMap { $0 }.joined(separator: " · ")
        case .cleared, nil:
            let own = subtitle(item, now: now)
            guard item.kind == .invite else { return own }
            return own.isEmpty ? "later" : "\(own) · later"
        }
    }

    public static func spokenTime(_ at: Date, now: Date, calendar: Calendar) -> String {
        let seconds = now.timeIntervalSince(at)
        func unit(_ value: Int, _ name: String) -> String { "\(value) \(name)\(value == 1 ? "" : "s") ago" }
        if seconds < 60 { return "just now" }
        if seconds < 3600 { return unit(Int(seconds / 60), "minute") }
        if seconds < 86400 { return unit(Int(seconds / 3600), "hour") }
        if seconds < 7 * 86400 { return unit(Int(seconds / 86400), "day") }
        return relativeTime(at, now: now, calendar: calendar)
    }

    public static func accessibilityLabel(_ item: Item, now: Date, calendar: Calendar) -> String {
        var head = rowTitle(item)
        switch item.kind {
        case .reply, .mention:
            if let cabal = item.cabal?.name { head += " in \(cabal)" }
            if let proposal = item.proposal { head += ", \(proposalTitle(proposal))" }
            if let quote = item.quote {
                let spoken = quote.text.replacingOccurrences(of: ": @", with: ", at ")
                    .replacingOccurrences(of: "@", with: "at ")
                head += ": \(spoken)"
            }
        case .waitingVote, .result, .invite:
            head += ", \(subtitle(item, now: now))"
        }
        let when = spokenTime(item.at, now: now, calendar: calendar) + (item.unread ? ", unread" : "")
        let separator = [".", "?", "!"].contains(where: head.hasSuffix) ? " " : ". "
        return head + separator + when + "."
    }

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

    public static func replySubtitle(_ item: Item) -> String {
        let cabal = item.cabal?.name
        let about: String?
        if let proposal = item.proposal {
            about = proposalTitle(proposal)
        } else if let post = item.post {
            about = post.actorName.isEmpty ? post.label : "\(post.actorName)'s \(post.label)"
        } else {
            about = nil
        }
        return [cabal, about].compactMap { $0 }.joined(separator: " · ")
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
}
