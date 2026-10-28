import Foundation

public struct ShareEyes: Equatable, Sendable {
    public var pnlDollars: Bool
    public var buySell: Bool
    public var code: Bool

    public init(pnlDollars: Bool = true, buySell: Bool = true, code: Bool = true) {
        self.pnlDollars = pnlDollars
        self.buySell = buySell
        self.code = code
    }
}

public enum ShareFormat: String, CaseIterable, Sendable {
    case post
    case square
    case story
    case sticker
}

public struct ShareCrest: Equatable, Sendable {
    public let cabalID: String
    public let name: String
    public let pictureURL: URL?

    public init(cabalID: String, name: String, pictureURL: URL?) {
        self.cabalID = cabalID
        self.name = name
        self.pictureURL = pictureURL
    }
}

public enum ShareMark: Equatable, Sendable {
    case crest(ShareCrest)
    case photo(url: URL?, name: String, crest: ShareCrest?)
}

public struct ShareMarker: Equatable, Sendable {
    public enum Kind: Equatable, Sendable {
        case buy
        case sell
        case seal
    }

    public let kind: Kind
    public let time: Date
    public let priceMicros: Int64
    public let callout: String?

    public init(kind: Kind, time: Date, priceMicros: Int64, callout: String? = nil) {
        self.kind = kind
        self.time = time
        self.priceMicros = priceMicros
        self.callout = callout
    }
}

public struct ShareChartPoint: Equatable, Sendable {
    public let time: Date
    public let priceMicros: Int64

    public init(time: Date, priceMicros: Int64) {
        self.time = time
        self.priceMicros = priceMicros
    }
}

public struct ShareChart: Equatable, Sendable {
    public let points: [ShareChartPoint]
    public let markers: [ShareMarker]

    public var callouts: [ShareMarker] { markers.filter { $0.callout != nil } }

    public init(points: [ShareChartPoint], markers: [ShareMarker]) {
        self.points = points
        self.markers = markers
    }
}

public struct ShareFacts: Equatable, Sendable {
    public enum Side: Equatable, Sendable {
        case long
        case short
    }

    public let symbol: String?
    public let firstBoughtAt: Date?
    public let side: Side?

    public init(symbol: String? = nil, firstBoughtAt: Date? = nil, side: Side? = nil) {
        self.symbol = symbol
        self.firstBoughtAt = firstBoughtAt
        self.side = side
    }
}

public enum ShareFigureTone: Equatable, Sendable {
    case gain
    case loss
}

public struct ShareCardSpec: Equatable, Sendable {
    public struct Header: Equatable, Sendable {
        public let mark: ShareMark
        public let title: String
        public let subtitle: String
        public let dateText: String

        public init(mark: ShareMark, title: String, subtitle: String, dateText: String) {
            self.mark = mark
            self.title = title
            self.subtitle = subtitle
            self.dateText = dateText
        }
    }

    public struct TickerLine: Equatable, Sendable {
        public let logoURL: URL?
        public let symbol: String
        public let detail: String

        public init(logoURL: URL?, symbol: String, detail: String) {
            self.logoURL = logoURL
            self.symbol = symbol
            self.detail = detail
        }
    }

    public struct Figure: Equatable, Sendable {
        public let main: String
        public let secondary: String?
        public let context: String?
        public let tone: ShareFigureTone

        public init(main: String, secondary: String?, context: String?, tone: ShareFigureTone) {
            self.main = main
            self.secondary = secondary
            self.context = context
            self.tone = tone
        }
    }

    public struct Stat: Equatable, Sendable {
        public let label: String
        public let value: String

        public init(label: String, value: String) {
            self.label = label
            self.value = value
        }
    }

    public let header: Header
    public let chart: ShareChart
    public let tickerLine: TickerLine
    public let figure: Figure
    public let stats: [Stat]
    public let code: String?
    public let replayTitle: String
    public let facts: ShareFacts
    public let spoken: String

    public init(
        header: Header, chart: ShareChart, tickerLine: TickerLine, figure: Figure, stats: [Stat], code: String?,
        replayTitle: String, facts: ShareFacts, spoken: String
    ) {
        self.header = header
        self.chart = chart
        self.tickerLine = tickerLine
        self.figure = figure
        self.stats = stats
        self.code = code
        self.replayTitle = replayTitle
        self.facts = facts
        self.spoken = spoken
    }
}
