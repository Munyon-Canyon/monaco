import SwiftUI
import UIKit

nonisolated struct PixelBitmap: Equatable, Hashable, Sendable {
    static let grid = 16

    let rows: [UInt16]

    init(_ art: [String]) {
        precondition(art.count == Self.grid, "a pixel bitmap has \(Self.grid) rows, got \(art.count)")
        rows = art.map { row in
            precondition(row.count == Self.grid, "a pixel bitmap row has \(Self.grid) pixels: \(row)")
            var bits: UInt16 = 0
            for (column, character) in row.enumerated() where character == "#" {
                bits |= 1 << UInt16(Self.grid - 1 - column)
            }
            return bits
        }
    }

    init(rows: [UInt16]) {
        precondition(rows.count == Self.grid)
        self.rows = rows
    }

    func isLit(column: Int, row: Int) -> Bool {
        rows[row] & (1 << UInt16(Self.grid - 1 - column)) != 0
    }

    var litCount: Int {
        rows.reduce(0) { $0 + $1.nonzeroBitCount }
    }

    var isEmpty: Bool { rows.allSatisfy { $0 == 0 } }

    func subtracting(_ other: PixelBitmap) -> PixelBitmap {
        PixelBitmap(rows: zip(rows, other.rows).map { $0 & ~$1 })
    }

    func union(_ other: PixelBitmap) -> PixelBitmap {
        PixelBitmap(rows: zip(rows, other.rows).map { $0 | $1 })
    }

    var flippedVertically: PixelBitmap {
        PixelBitmap(rows: rows.reversed())
    }

    var flippedHorizontally: PixelBitmap {
        PixelBitmap(
            rows: rows.map { row in
                var mirrored: UInt16 = 0
                for column in 0..<Self.grid where row & (1 << UInt16(Self.grid - 1 - column)) != 0 {
                    mirrored |= 1 << UInt16(column)
                }
                return mirrored
            })
    }

    var runs: [(row: Int, start: Int, length: Int)] {
        var runs: [(row: Int, start: Int, length: Int)] = []
        for row in 0..<Self.grid {
            var column = 0
            while column < Self.grid {
                guard isLit(column: column, row: row) else {
                    column += 1
                    continue
                }
                let start = column
                while column < Self.grid, isLit(column: column, row: row) { column += 1 }
                runs.append((row, start, column - start))
            }
        }
        return runs
    }

    static func unit(fitting rect: CGRect, scale: CGFloat) -> CGFloat {
        let raw = min(rect.width, rect.height) / CGFloat(grid)
        guard scale > 0 else { return raw }
        return max(floor(raw * scale) / scale, 1 / scale)
    }

    func path(in rect: CGRect, scale: CGFloat) -> Path {
        let unit = Self.unit(fitting: rect, scale: scale)
        let side = unit * CGFloat(Self.grid)
        let snap = { (value: CGFloat) -> CGFloat in scale > 0 ? (value * scale).rounded() / scale : value }
        let originX = snap(rect.midX - side / 2)
        let originY = snap(rect.midY - side / 2)
        var path = Path()
        for run in runs {
            path.addRect(
                CGRect(
                    x: originX + CGFloat(run.start) * unit,
                    y: originY + CGFloat(run.row) * unit,
                    width: CGFloat(run.length) * unit,
                    height: unit))
        }
        return path
    }

    func templateImage(pointSize: CGFloat, scale: CGFloat) -> UIImage {
        let format = UIGraphicsImageRendererFormat()
        format.scale = scale
        format.opaque = false
        let size = CGSize(width: pointSize, height: pointSize)
        let image = UIGraphicsImageRenderer(size: size, format: format).image { context in
            context.cgContext.setShouldAntialias(false)
            context.cgContext.setFillColor(UIColor.black.cgColor)
            context.cgContext.addPath(path(in: CGRect(origin: .zero, size: size), scale: scale).cgPath)
            context.cgContext.fillPath()
        }
        return image.withRenderingMode(.alwaysTemplate)
    }
}

struct PixelGlyphShape: Shape {
    let bitmap: PixelBitmap
    var scale: CGFloat = 3

    func path(in rect: CGRect) -> Path {
        bitmap.path(in: rect, scale: scale)
    }
}

struct MonacoGlyph: View {
    let icon: MonacoIcon
    var filled = false
    var size: CGFloat = MonacoGlyphMetrics.regular
    var weight: Font.Weight = .medium

    @Environment(\.displayScale) private var displayScale

    init(
        _ icon: MonacoIcon, filled: Bool = false, size: CGFloat = MonacoGlyphMetrics.regular,
        weight: Font.Weight = .medium
    ) {
        self.icon = icon
        self.filled = filled
        self.size = size
        self.weight = weight
    }

    var body: some View {
        Group {
            if let bitmap = icon.pixel(filled: filled) {
                PixelGlyphShape(bitmap: bitmap, scale: displayScale)
                    .fill(style: FillStyle(antialiased: false))
            } else {
                Image(systemName: filled ? icon.filled : icon.symbol)
                    .font(.system(size: size, weight: weight))
            }
        }
        .frame(width: size, height: size)
        .accessibilityHidden(true)
    }
}

enum MonacoGlyphMetrics {
    static let small: CGFloat = 16
    static let regular: CGFloat = 22
    static let large: CGFloat = 30
    static let tab: CGFloat = 25
}
