import Foundation
import MonacoAPI

extension FeedPostFacts {
    struct ResultReading {
        let verb: Spoken<String>
        let object: Line?
        let attachment: Spoken<Attachment>
        let possessive: Bool
        let sentence: ((_ name: String, _ age: String) -> String)?

        init?(_ item: Components.Schemas.FeedItem, now: Date) {
            guard let outcome = item.outcome else { return nil }
            let order = Order(item, passed: outcome == .passed)
            verb = order.verb
            object = order.object
            let attachment = order.attachment(now: now)
            self.attachment = attachment
            possessive = order.callsIt
            sentence =
                order.callsIt
                ? { name, age in order.callSentence(name: name, age: age, attachment: attachment.value) } : nil
        }
    }

    fileprivate enum OrderState {
        case noTrade, pending, partial, failed, filled
    }

    fileprivate struct Order {
        let item: Components.Schemas.FeedItem
        let state: OrderState
        let isSell: Bool
        let isPreIPO: Bool
        let leadsWithCabal: Bool
        let callsIt: Bool

        init(_ item: Components.Schemas.FeedItem, passed: Bool) {
            self.item = item
            isSell = item.action == "sell"
            isPreIPO = item.assetKind == .preIpo
            leadsWithCabal = item.actorId == nil
            switch (passed, item.fillState) {
            case (false, _): state = .noTrade
            case (true, .filled): state = .filled
            case (true, .partial): state = .partial
            case (true, .failed): state = .failed
            default: state = .pending
            }
            callsIt = item.shape == .cabal && item.actorId != nil && !(state == .filled && (isSell || isPreIPO))
        }

        var isNamed: Bool { state == .filled && (isSell || isPreIPO) }

        private var ticker: String? {
            guard let symbol = FeedPostFacts.text(item.symbol) else { return nil }
            return isPreIPO
                ? FeedPostFacts.text(item.assetName) ?? AssetSymbolFormatter.display(symbol, kind: .preIpo)
                : AssetSymbolFormatter.display(symbol)
        }

        private var callVerb: String {
            let tally = item.tally.map { ", \($0.yes) to \($0.no)" } ?? ""
            let word = isSell ? "sale" : "buy"
            let outcome = state == .noTrade ? "failed" : "passed"
            return [ticker, word, outcome].compactMap { $0 }.joined(separator: " ") + tally
        }

        var verb: Spoken<String> {
            if callsIt { return Spoken(value: callVerb, spoken: callVerb) }
            let tally = item.tally.map { " \($0.yes)–\($0.no)" } ?? ""
            switch state {
            case .noTrade: return Spoken(value: "failed it\(tally)", spoken: "failed")
            case .filled where isNamed:
                let word = isSell ? "sold" : "bought"
                return Spoken(value: word, spoken: word)
            default: return Spoken(value: "passed it\(tally)", spoken: "passed")
            }
        }

        var object: Line? {
            guard let symbol = FeedPostFacts.text(item.symbol) else { return nil }
            let ticker = ticker ?? symbol
            let action = isNamed ? nil : item.action.map(FeedPostFacts.capitalized)
            let name = [action, ticker].compactMap { $0 }.joined(separator: " ")
            let figure = FeedPostFacts.amount(of: item)
            let place = place
            let line = Object(
                symbol: symbol, logoURL: FeedPostFacts.text(item.assetLogoUrl),
                headline: [name, figure?.short].compactMap { $0 }.joined(separator: " "), place: place.text,
                trailing: trailing, placeIsWarning: place.isWarning)
            var spoken = [name, figure?.spoken].compactMap { $0 }.joined(separator: ", ")
            if !isNamed, let tally = item.tally { spoken += ", \(tally.yes) to \(tally.no)" }
            if !leadsWithCabal, let cabal = FeedPostFacts.text(item.cabalName) { spoken += ", in \(cabal)" }
            return Line(line: line, spoken: spoken)
        }

        private var place: (text: String?, isWarning: Bool) {
            switch state {
            case .noTrade: (FeedPostFacts.text(item.cabalName), false)
            case .pending: (isSell ? "Selling now" : "Buying now", false)
            case .partial: ("Half filled", true)
            case .failed: ("Didn\u{2019}t go through", true)
            case .filled: (FeedPostFacts.text(item.cabalName), false)
            }
        }

        func callSentence(name: String, age: String, attachment: Attachment) -> String {
            let order = [item.action.map(FeedPostFacts.capitalized), ticker, FeedPostFacts.amount(of: item)?.spoken]
                .compactMap { $0 }.joined(separator: " ")
            var parts = [order, place.text].compactMap { $0 }
            if state == .filled, let bps = item.returnBps {
                let move = Self.spokenMove(bps, basis: "since the fill").dropLast()
                parts.append(move.prefix(1).lowercased() + move.dropFirst())
            }
            var sentences = ["\(name) \(callVerb), \(age).", parts.joined(separator: ", ") + "."]
            if case .result(let caller, _, _) = attachment { sentences.append(ResultCopy.calledIt(caller) + ".") }
            return sentences.joined(separator: " ")
        }

        private var trailing: Trailing? {
            guard state == .filled else { return nil }
            if isPreIPO { return .text("Pre-IPO") }
            return item.returnBps.map { .figure($0) }
        }

        func attachment(now: Date) -> Spoken<Attachment> {
            switch state {
            case .noTrade: return Spoken(value: .none, spoken: "")
            case .pending: return Spoken(value: .none, spoken: isSell ? "Selling now." : "Buying now.")
            case .partial: return Spoken(value: .none, spoken: "Half filled.")
            case .failed: return Spoken(value: .none, spoken: "Didn\u{2019}t go through.")
            case .filled: return filled(now: now)
            }
        }

        private func filled(now: Date) -> Spoken<Attachment> {
            if isPreIPO { return Spoken(value: .none, spoken: "Filled. Pre-IPO.") }
            let isLoss = (item.returnBps ?? 0) < 0
            let basis = isSell ? "on what the cabal paid" : "since the fill"
            let move = item.returnBps.map { Self.spokenMove($0, basis: basis) }
            guard isSell else {
                let called = item.caller.map { ResultCopy.calledIt($0.name) + "." }
                let spoken = ["Filled.", move, called].compactMap { $0 }.joined(separator: " ")
                guard let caller = item.caller else { return Spoken(value: .none, spoken: spoken) }
                let points = chartPoints
                return Spoken(
                    value: .result(sealName: caller.name, sparkline: points, isLoss: isLoss), spoken: spoken)
            }
            let markers = chartMarkers
            let dates = Self.spokenDates(markers, now: now)
            let spoken = [move, dates].compactMap { $0 }.joined(separator: " ")
            let points = chartPoints
            guard points.count >= 2 else { return Spoken(value: .none, spoken: spoken) }
            return Spoken(value: .sale(points: points, markers: markers, isLoss: isLoss), spoken: spoken)
        }

        private var chartPoints: [ChartPoint] {
            (item.chart?.points ?? []).map { ChartPoint(at: $0.at, priceMicros: $0.priceMicros) }
        }

        private var chartMarkers: [ChartMarker] {
            (item.chart?.markers ?? []).map {
                ChartMarker(at: $0.at, side: $0.side == .sell ? .sell : .buy, priceMicros: $0.priceMicros)
            }
        }

        private static func spokenMove(_ bps: Int64, basis: String) -> String {
            let magnitude = PercentFormatter.format(
                basisPoints: Int64(clamping: bps.magnitude), signed: false, fractionDigits: 1)
            let number = magnitude.dropLast()
            switch bps {
            case 0: return "Unchanged \(basis)."
            case ..<0: return "Down \(number) percent \(basis)."
            default: return "Up \(number) percent \(basis)."
            }
        }

        private static func spokenDates(_ markers: [ChartMarker], now: Date) -> String? {
            var phrases: [String] = []
            if let buy = markers.first(where: { $0.side == .buy }) {
                phrases.append("bought \(day(buy.at, now: now))")
            }
            if let sell = markers.last(where: { $0.side == .sell }) {
                phrases.append("sold \(day(sell.at, now: now))")
            }
            guard let first = phrases.first else { return nil }
            return ([first.prefix(1).uppercased() + first.dropFirst()] + phrases.dropFirst())
                .joined(separator: ", ") + "."
        }

        private static let months = [
            "January", "February", "March", "April", "May", "June", "July", "August", "September", "October",
            "November", "December",
        ]

        private static func day(_ date: Date, now: Date) -> String {
            let calendar = Calendar.current
            if calendar.isDate(date, inSameDayAs: now) { return "today" }
            let parts = calendar.dateComponents([.month, .day], from: date)
            guard let month = parts.month, let day = parts.day else { return "earlier" }
            return "\(months[month - 1]) \(day)"
        }
    }
}
