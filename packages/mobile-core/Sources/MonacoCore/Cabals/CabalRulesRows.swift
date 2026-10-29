import Foundation
import MonacoAPI

public struct CabalRulesRows: Equatable, Sendable {
    public enum Kind: String, CaseIterable, Sendable {
        case votesToPass, voteWindow, maxPerTrade, whoCanPropose, cashOut, showSides
    }

    public struct Row: Equatable, Sendable, Identifiable {
        public let kind: Kind
        public let title: String
        public let value: String
        public var id: Kind { kind }
    }

    public static let footerPrefix =
        "Counts are live for every member. With this on, names by side show after a vote closes. Rules change by proposal; "

    public let rows: [Row]
    public let footer: String
    public let showSides: Bool

    public init(
        _ cabal: Components.Schemas.Cabal, now: Date, calendar: Calendar = .current, locale: Locale = .current
    ) {
        let rules = cabal.rules
        showSides = rules.showSides
        rows = [
            Row(kind: .votesToPass, title: "Votes to pass", value: Self.votesToPass(cabal)),
            Row(kind: .voteWindow, title: "Vote window", value: Self.window(rules.proposalExpirySeconds)),
            Row(kind: .maxPerTrade, title: "Max per trade", value: Self.maxPerTrade(rules.maxTradeBps)),
            Row(kind: .whoCanPropose, title: "Who can propose", value: "Voters"),
            Row(kind: .cashOut, title: "Cash out", value: "Anytime, your share"),
            Row(
                kind: .showSides, title: "Show who voted what",
                value: Self.showSidesValue(rules, now: now, calendar: calendar, locale: locale)),
        ]
        footer = Self.footerPrefix + Self.adminSentence(cabal) + "."
    }

    static func votesToPass(_ cabal: Components.Schemas.Cabal) -> String {
        let voters = Int(cabal.voterCount)
        let needed = CabalThreshold(rawValue: cabal.rules.threshold) == .unanimous ? voters : voters / 2 + 1
        return "\(needed) of \(voters)"
    }

    static func window(_ seconds: Int32) -> String {
        switch CabalProposalExpiry(rawValue: seconds) {
        case .oneHour: "1 h"
        case .oneDay: "24 h"
        case .oneWeek: "7 d"
        case nil: CabalRulesSummary.durationLabel(seconds: seconds)
        }
    }

    static func maxPerTrade(_ bps: Int32?) -> String {
        guard let bps else { return "No limit" }
        let whole = bps / 100
        let hundredths = bps % 100
        guard hundredths != 0 else { return "\(whole)% of cash" }
        let padded = hundredths < 10 ? "0\(hundredths)" : "\(hundredths)"
        return "\(whole).\(padded)% of cash"
    }

    static func showSidesValue(_ rules: Components.Schemas.CabalRules, now: Date, calendar: Calendar, locale: Locale)
        -> String
    {
        let base = "After a vote closes \u{00B7} "
        guard rules.showSides else { return base + "off" }
        guard let since = rules.showSidesSince else { return base + "on" }
        return base + "on since " + dayLabel(since, now: now, calendar: calendar, locale: locale)
    }

    static func dayLabel(_ date: Date, now: Date, calendar: Calendar, locale: Locale) -> String {
        let sameYear = calendar.component(.year, from: date) == calendar.component(.year, from: now)
        let format: Date.FormatStyle = {
            let base = Date.FormatStyle(locale: locale, calendar: calendar, timeZone: calendar.timeZone)
            return sameYear ? base.month(.abbreviated).day() : base.month(.abbreviated).day().year()
        }()
        return date.formatted(format)
    }

    static func adminSentence(_ cabal: Components.Schemas.Cabal) -> String {
        let listed = cabal.admins ?? []
        let people = listed.isEmpty ? [cabal.creator] : listed
        let names = people.map { $0.displayName.isEmpty ? ($0.handle ?? "someone") : $0.displayName }
        guard names.count > 1, let last = names.last else { return "the admin is \(names.first ?? "")" }
        return "admins are " + names.dropLast().joined(separator: ", ") + " and " + last
    }
}
