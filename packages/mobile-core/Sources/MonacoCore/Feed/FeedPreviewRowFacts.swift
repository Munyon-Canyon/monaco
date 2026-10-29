import Foundation
import MonacoAPI

public struct FeedPreviewRowFacts: Equatable, Sendable, Identifiable {
    public enum Lead: Equatable, Sendable {
        case person(name: String, photoURL: String?, userID: String)
        case cabal(name: String, pictureURL: String?, cabalID: String?)
        case asset(symbol: String, name: String?, logoURL: String?, isPreIPO: Bool)
    }

    public enum Trailing: Equatable, Sendable {
        case split(yes: Int, no: Int)
        case amount(dollars: String?, bps: Int64?)
        case symbol(String, bps: Int64?)
        case label(String)

        public var returnText: String? {
            switch self {
            case .amount(_, let bps?), .symbol(_, let bps?):
                PercentFormatter.format(basisPoints: bps, signed: true, fractionDigits: 1)
            case .split, .amount, .symbol, .label:
                nil
            }
        }
    }

    public let id: String
    public let destination: FeedDestination
    public let lead: Lead
    public let title: String
    public let subtitleParts: [String]
    public let replies: Int
    public let trailing: Trailing?
    public let spoken: String

    public var subtitle: String { subtitleParts.joined(separator: " · ") }

    public init(_ item: Components.Schemas.FeedItem, source: HomeFeedPreviewModel.Source, now: Date) {
        let facts = FeedPostFacts(item, now: now)
        let reading = Reading(item: item, facts: facts, source: source)
        let copy = reading.copy
        id = item.id
        destination = FeedDestination.destination(for: item)
        lead = reading.lead
        title = copy.title
        subtitleParts = copy.parts
        replies = Int(item.replyCount)
        trailing = copy.trailing
        spoken = Self.spoken(title: copy.title, parts: copy.parts, trailing: copy.trailing, replies: replies)
    }

    private static func spoken(title: String, parts: [String], trailing: Trailing?, replies: Int) -> String {
        var sentence = [title, parts.joined(separator: ", ")].filter { !$0.isEmpty }
        switch trailing {
        case .split(let yes, let no): sentence.append("\(yes) yes, \(no) no")
        case .amount(let dollars, _): sentence += [dollars, trailing?.returnText].compactMap { $0 }
        case .symbol(let symbol, _): sentence += [symbol, trailing?.returnText].compactMap { $0 }
        case .label(let text): sentence.append(text)
        case nil: break
        }
        if replies > 0 { sentence.append("\(replies) \(replies == 1 ? "reply" : "replies")") }
        return sentence.joined(separator: ", ")
    }
}

extension FeedPreviewRowFacts {
    fileprivate struct Copy {
        let title: String
        let parts: [String]
        let trailing: Trailing?
    }

    fileprivate struct Reading {
        let item: Components.Schemas.FeedItem
        let facts: FeedPostFacts
        let source: HomeFeedPreviewModel.Source

        var isEveryone: Bool { source == .everyone }
        var cabal: String? { FeedPostFacts.text(item.cabalName) }
        var isSell: Bool { item.action == "sell" }
        var isPreIPO: Bool { item.assetKind == .preIpo }
        var ticker: String? { item.symbol.flatMap(FeedPostFacts.text).map { AssetSymbolFormatter.display($0) } }
        var order: String {
            [item.action.map(FeedPostFacts.capitalized), ticker].compactMap { $0 }.joined(separator: " ")
        }

        var assetLead: Lead? {
            guard let symbol = FeedPostFacts.text(item.symbol) else { return nil }
            return .asset(
                symbol: symbol, name: FeedPostFacts.text(item.assetName),
                logoURL: FeedPostFacts.text(item.assetLogoUrl),
                isPreIPO: isPreIPO)
        }

        var lead: Lead {
            switch FeedPostKind(item.kind) {
            case .result:
                if item.outcome == .failed { return assetLead ?? factsLead }
                if isSell && !isPreIPO { return assetLead ?? factsLead }
                if let caller = item.caller, !isPreIPO {
                    return .person(name: caller.name, photoURL: FeedPostFacts.text(caller.photoUrl), userID: caller.id)
                }
                return factsLead
            default:
                return factsLead
            }
        }

        var factsLead: Lead {
            switch facts.lead {
            case .person(let name, let photoURL, let userID): .person(name: name, photoURL: photoURL, userID: userID)
            case .cabal(let name, let pictureURL, let cabalID):
                .cabal(name: name, pictureURL: pictureURL, cabalID: cabalID)
            }
        }

        var copy: Copy {
            switch FeedPostKind(item.kind) {
            case .proposal: proposal
            case .result: result
            case .reply: reply
            case .memberJoined: Copy(title: "\(facts.name) joined", parts: placed, trailing: nil)
            case .cabalCreated, .trade, .threshold, .other: Copy(title: item.title, parts: placed, trailing: nil)
            }
        }

        var placed: [String] { [cabal, facts.age].compactMap { $0 } }

        var split: Trailing? { item.tally.map { .split(yes: $0.yes, no: $0.no) } }

        var proposal: Copy {
            let headline = facts.object?.headline ?? item.title
            guard case .tally(let tally) = facts.attachment else {
                return Copy(title: headline, parts: placed, trailing: split)
            }
            if isEveryone {
                return Copy(title: headline, parts: [facts.name, cabal, facts.age].compactMap { $0 }, trailing: split)
            }
            return Copy(
                title: [cabal, headline].compactMap { $0 }.joined(separator: " · "), parts: [tally.statusLine],
                trailing: split)
        }

        var reply: Copy {
            var parts = [facts.age]
            if case .quote(let quote) = facts.attachment { parts.insert(quote, at: 0) }
            return Copy(title: "\(facts.name) \(facts.verb)", parts: parts, trailing: nil)
        }

        var result: Copy {
            let shown = ticker ?? item.title
            guard item.outcome == .passed else {
                let tally = item.tally.map { " \($0.yes)–\($0.no)" } ?? ""
                return Copy(
                    title: "\(order) failed\(tally)", parts: [cabal, "no trade", facts.age].compactMap { $0 },
                    trailing: split)
            }
            guard item.fillState == .filled else {
                let status = isSell ? "selling" : "buying"
                return Copy(
                    title: "\(order) passed", parts: [cabal, status, facts.age].compactMap { $0 }, trailing: split)
            }
            if isPreIPO { return preIPO }
            if isSell { return sale(ticker: shown) }
            return purchase(ticker: shown)
        }

        var preIPO: Copy {
            let name =
                FeedPostFacts.text(item.assetName)
                ?? item.symbol.map { AssetSymbolFormatter.display($0, kind: .preIpo) }
                ?? item.title
            let figure = isEveryone ? FeedPostFacts.amount(of: item)?.short : nil
            let title = ["\(facts.name) bought \(name)", figure].compactMap { $0 }.joined(separator: " ")
            return Copy(title: title, parts: placed, trailing: .label("Pre-IPO"))
        }

        func sale(ticker: String) -> Copy {
            let dollars = FeedPostFacts.amount(of: item)?.short
            if isEveryone {
                let title = ["\(facts.name) sold \(ticker)", dollars].compactMap { $0 }.joined(separator: " ")
                return Copy(title: title, parts: placed, trailing: .amount(dollars: nil, bps: item.returnBps))
            }
            let line: String? = item.caller.map {
                (item.returnBps ?? 0) > 0 ? ResultCopy.calledTheTop($0.name) : ResultCopy.call($0.name)
            }
            return Copy(
                title: "\(cabal ?? facts.name) sold \(ticker)", parts: [line, facts.age].compactMap { $0 },
                trailing: .amount(dollars: dollars, bps: item.returnBps))
        }

        func purchase(ticker: String) -> Copy {
            let title = item.caller.map { ResultCopy.calledIt($0.name) } ?? "\(cabal ?? facts.name) bought \(ticker)"
            return Copy(
                title: title, parts: [order, cabal, facts.age].compactMap { $0 },
                trailing: .symbol(ticker, bps: item.returnBps))
        }
    }
}
