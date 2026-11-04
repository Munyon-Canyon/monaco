#!/usr/bin/env swift
// Prints the SceneColor table body (one 0xRRGGBB row per cover) from the covers in
// apps/mobile/Monaco/Assets.xcassets/CabalCovers, or from the directory given as argument 1.
// Each cover gets its dominant hue at cover-01's saturation and lightness; cover-01 stays 0x0E2A3D.
import AppKit

let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
    .deletingLastPathComponent()
let dir =
    CommandLine.arguments.count > 1
    ? CommandLine.arguments[1]
    : root.appendingPathComponent("apps/mobile/Monaco/Assets.xcassets/CabalCovers").path

func hsl(_ r: Double, _ g: Double, _ b: Double) -> (Double, Double, Double) {
    let mx = max(r, g, b)
    let mn = min(r, g, b)
    let l = (mx + mn) / 2
    if mx == mn { return (0, 0, l) }
    let d = mx - mn
    let s = l > 0.5 ? d / (2 - mx - mn) : d / (mx + mn)
    var h: Double
    if mx == r {
        h = (g - b) / d + (g < b ? 6 : 0)
    } else if mx == g {
        h = (b - r) / d + 2
    } else {
        h = (r - g) / d + 4
    }
    return (h / 6, s, l)
}

func rgb(_ h: Double, _ s: Double, _ l: Double) -> (Double, Double, Double) {
    func f(_ p: Double, _ q: Double, _ t0: Double) -> Double {
        var t = t0
        if t < 0 { t += 1 }
        if t > 1 { t -= 1 }
        if t < 1 / 6 { return p + (q - p) * 6 * t }
        if t < 1 / 2 { return q }
        if t < 2 / 3 { return p + (q - p) * (2 / 3 - t) * 6 }
        return p
    }
    let q = l < 0.5 ? l * (1 + s) : l + s - l * s
    let p = 2 * l - q
    return (f(p, q, h + 1 / 3), f(p, q, h), f(p, q, h - 1 / 3))
}

let (_, s0, l0) = hsl(14 / 255, 42 / 255, 61 / 255)
let bins = 36
for i in 1...26 {
    if i == 1 {
        print("0x0E2A3D,")
        continue
    }
    let name = String(format: "cover-%02d", i)
    let url = URL(fileURLWithPath: "\(dir)/\(name).imageset/\(name)@2x.jpg")
    let rep = NSBitmapImageRep(data: try! Data(contentsOf: url))!
    let small = NSBitmapImageRep(
        bitmapDataPlanes: nil, pixelsWide: 128, pixelsHigh: 72, bitsPerSample: 8, samplesPerPixel: 4,
        hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: small)
    rep.draw(in: NSRect(x: 0, y: 0, width: 128, height: 72))
    var hist = [Double](repeating: 0, count: bins)
    for x in 0..<128 {
        for y in 0..<72 {
            let c = small.colorAt(x: x, y: y)!.usingColorSpace(.sRGB)!
            let (h, s, _) = hsl(c.redComponent, c.greenComponent, c.blueComponent)
            if s > 0.35 && (h * 360 < 15 || h * 360 > 60) {
                hist[min(bins - 1, Int(h * Double(bins)))] += s
            }
        }
    }
    let k = hist.indices.max { hist[$0] < hist[$1] }!
    let (r, g, b) = rgb((Double(k) + 0.5) / Double(bins), s0, l0)
    print(
        String(
            format: "0x%02X%02X%02X,", Int((r * 255).rounded()), Int((g * 255).rounded()),
            Int((b * 255).rounded())))
}
