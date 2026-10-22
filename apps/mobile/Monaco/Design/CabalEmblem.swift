import SwiftUI

nonisolated enum CabalEmblem: String, CaseIterable, Sendable {
    case alien, bulldog, chihuahua, bob, curls, ponytail, headband, glasses

    var spokenName: String {
        switch self {
        case .alien: "the alien"
        case .bulldog: "the bulldog"
        case .chihuahua: "the chihuahua"
        case .bob: "the chair"
        case .curls: "the wildcard"
        case .ponytail: "the captain"
        case .headband: "the backtest"
        case .glasses: "the risk officer"
        }
    }

    static func forGroupId(_ groupId: String) -> CabalEmblem {
        let key = groupId.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let hash = MonacoTheme.CabalTint.fnv1a64("emblem:" + key)
        return allCases[Int(hash % UInt64(allCases.count))]
    }

    var bitmap: PixelBitmap {
        switch self {
        case .alien: CabalEmblemArt.alien
        case .bulldog: CabalEmblemArt.bulldog
        case .chihuahua: CabalEmblemArt.chihuahua
        case .bob: CabalEmblemArt.bob
        case .curls: CabalEmblemArt.curls
        case .ponytail: CabalEmblemArt.ponytail
        case .headband: CabalEmblemArt.headband
        case .glasses: CabalEmblemArt.glasses
        }
    }
}

nonisolated enum CabalEmblemArt {
    static let alien = PixelBitmap([
        "....#......#....",
        "....#......#....",
        ".....#....#.....",
        "....########....",
        "..############..",
        ".##############.",
        ".##..######..##.",
        ".#....####....#.",
        ".##..######..##.",
        ".##############.",
        "..############..",
        "...##########...",
        "....########....",
        "......####......",
        "...##..##..##...",
        "..############..",
    ])

    static let bulldog = PixelBitmap([
        "................",
        ".....######.....",
        "...##########...",
        "..############..",
        ".##############.",
        ".##############.",
        ".##.########.##.",
        ".##.########.##.",
        ".##############.",
        ".######..######.",
        ".#####....#####.",
        "..###.####.###..",
        "..####....####..",
        "...##########...",
        ".....######.....",
        "................",
    ])

    static let chihuahua = PixelBitmap([
        "..##........##..",
        "..###......###..",
        "..####....####..",
        "..#####..#####..",
        "..############..",
        "..############..",
        "..##..####..##..",
        "..############..",
        "..############..",
        "...####..####...",
        "...##########...",
        "....########....",
        ".....######.....",
        "......####......",
        "................",
        "................",
    ])

    static let bob = PixelBitmap([
        "................",
        ".....######.....",
        "...##########...",
        "..############..",
        ".##############.",
        ".##############.",
        ".##..........##.",
        ".##..........##.",
        ".##############.",
        ".##############.",
        ".##..........##.",
        ".##..######..##.",
        ".##..........##.",
        ".##..........##.",
        "......####......",
        "................",
    ])

    static let curls = PixelBitmap([
        "....##....##....",
        "..############..",
        ".##############.",
        "################",
        "################",
        ".##..##..##..##.",
        "##..######..####",
        "##..######..####",
        ".##############.",
        "################",
        "###..######..###",
        "###..######..###",
        ".######..######.",
        "..####....####..",
        "...##......##...",
        "................",
    ])

    static let ponytail = PixelBitmap([
        "................",
        ".....######.....",
        "...##########...",
        "..############..",
        ".##############.",
        ".####......#####",
        ".###........####",
        ".###........####",
        ".###........####",
        ".###.##..##.####",
        ".###........###.",
        "..##........###.",
        "...#........##..",
        "....#......#....",
        ".....######.....",
        "................",
    ])

    static let headband = PixelBitmap([
        "................",
        ".....######.....",
        "...##########...",
        "..############..",
        ".##############.",
        ".#............##",
        ".#............##",
        ".##############.",
        ".##..######..##.",
        ".##..######..##.",
        ".##############.",
        ".##############.",
        "..####....####..",
        "...##########...",
        ".....######.....",
        "................",
    ])

    static let glasses = PixelBitmap([
        "................",
        ".....######.....",
        "...##########...",
        "..############..",
        ".##############.",
        ".##############.",
        "#....#....#....#",
        "#....#....#....#",
        ".####.####.####.",
        ".##############.",
        ".##############.",
        ".####......####.",
        "..###.####.###..",
        "...##########...",
        ".....######.....",
        "................",
    ])
}

struct CabalEmblemPattern: View {
    let emblem: CabalEmblem
    var color: Color = MonacoTheme.emblemPattern

    @Environment(\.displayScale) private var displayScale

    static let cell: CGFloat = 28
    static let glyph: CGFloat = 16
    static let gap: CGFloat = 20

    var body: some View {
        Canvas(opaque: false, rendersAsynchronously: false) { context, size in
            let stride = Self.cell + Self.gap
            let columns = Int(ceil(size.width / stride)) + 1
            let rows = Int(ceil(size.height / stride)) + 1
            for row in 0..<rows {
                let shift = row.isMultiple(of: 2) ? 0 : stride / 2
                for column in -1..<columns {
                    let origin = CGPoint(
                        x: CGFloat(column) * stride + shift + Self.gap / 2,
                        y: CGFloat(row) * stride + Self.gap / 2)
                    let cell = CGRect(origin: origin, size: CGSize(width: Self.cell, height: Self.cell))
                        .insetBy(dx: (Self.cell - Self.glyph) / 2, dy: (Self.cell - Self.glyph) / 2)
                    context.fill(
                        emblem.bitmap.path(in: cell, scale: displayScale), with: .color(color),
                        style: FillStyle(antialiased: false))
                }
            }
        }
        .accessibilityHidden(true)
    }
}
