import Foundation
import MonacoRanking

public enum ShareLedgerBuilder {
    public static func build(content: LedgerShareContent, eyes: ShareEyes) -> ShareCardSpec {
        let cover = content.cover
        let crest = ShareCrest(cabalID: cover.cabalID, name: cover.name, pictureURL: cover.pictureURL)
        let tone: ShareFigureTone = content.isLoss ? .loss : .gain
        let figure =
            eyes.pnlDollars
            ? ShareCardSpec.Figure(
                main: content.figure, secondary: nil, context: LedgerShareContent.figureContext, tone: tone)
            : ShareCardSpec.Figure(main: content.weekTitle, secondary: nil, context: nil, tone: tone)
        let stats: [ShareCardSpec.Stat] =
            eyes.buySell
            ? [
                .init(label: "Best call", value: content.bestCall),
                .init(label: LedgerCopy.sharpestLabel, value: content.sharpest),
                .init(label: LedgerCopy.votedLabel, value: content.voted),
            ] : []
        let code = eyes.code ? content.code : nil
        return ShareCardSpec(
            header: .init(
                mark: .photo(url: content.photoURL, name: content.name, crest: crest), title: content.header,
                subtitle: content.subline, dateText: ""),
            chart: chart(content, showsCallouts: eyes.buySell),
            tickerLine: .init(logoURL: nil, symbol: "", detail: ""), figure: figure, stats: stats, code: code,
            replayTitle: LedgerShareContent.replayTitle, facts: ShareFacts(isLedger: true),
            spoken: spoken(content, eyes: eyes, code: code))
    }

    static func chart(_ content: LedgerShareContent, showsCallouts: Bool) -> ShareChart {
        let points = content.points.map { ShareChartPoint(time: $0.time, priceMicros: $0.returnBps) }
        let markers: [ShareMarker] = content.markers.compactMap { marker in
            guard let nearest = points.min(by: { distance($0, marker) < distance($1, marker) }) else { return nil }
            switch marker.kind {
            case .call:
                return ShareMarker(
                    kind: .seal, time: marker.time, priceMicros: nearest.priceMicros,
                    callout: showsCallouts ? marker.label : nil)
            case .buy:
                return ShareMarker(kind: .buy, time: marker.time, priceMicros: nearest.priceMicros)
            }
        }
        return ShareChart(points: points, markers: markers)
    }

    private static func distance(_ point: ShareChartPoint, _ marker: LedgerShareContent.Marker) -> TimeInterval {
        abs(point.time.timeIntervalSince(marker.time))
    }

    static func spoken(_ content: LedgerShareContent, eyes: ShareEyes, code: String?) -> String {
        var parts = [content.name + ", " + content.weekTitle.lowercased()]
        if eyes.pnlDollars, let spokenReturn = content.spokenReturn { parts[0] += ", " + spokenReturn }
        if eyes.buySell {
            if let best = content.spokenBestCall { parts.append("Best call " + best) }
            parts.append("Sharpest " + content.sharpest)
            parts.append("Voted " + content.voted)
        }
        if let code { parts.append("Join with code " + code) }
        return parts.joined(separator: ". ") + "."
    }
}
