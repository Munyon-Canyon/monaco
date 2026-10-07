import MonacoCore
import SwiftUI

/// The top of the cabal screen: who this is, the pot, how it has been doing, and your slice
/// of it — one deep ink band across the whole width, in both schemes.
///
/// This is the one high-impact surface in the app, and it is the cabal's. The pot's curve
/// runs edge to edge inside it with the range under it; the cabal's tint is the rule along
/// the band's bottom edge and the mark at its top, never a full-bleed wash — five cabals in
/// five colours of ink would stop being one app.
struct GroupHeroSection: View {
    let view: GroupViewDTO
    /// Set, replace and remove the cabal picture. Nil on surfaces that only show
    /// the hero (the sample harness's read-only states), which then get a plain mark.
    var pictureEditor: CabalPictureEditor?
    var onPictureResult: (MonacoToast) -> Void = { _ in }

    private var tint: MonacoTheme.CabalTint { .forGroupId(view.id) }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            identity
            pot
            MonacoRule(color: MonacoTheme.onHeroHairline)
            slice
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MonacoTheme.heroInk)
        // The cabal's own colour, as the band's edge: enough to be this cabal's, not enough to
        // be a second canvas.
        .overlay(alignment: .bottom) {
            Rectangle()
                .fill(tint.onInk)
                .frame(height: 3)
                .accessibilityHidden(true)
        }
        .accessibilityIdentifier("group-hero")
    }

    // MARK: - Who

    private var identity: some View {
        HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
            if let pictureEditor {
                CabalPicturePicker(
                    groupId: view.id,
                    name: view.name,
                    canEdit: view.viewerIsCreator,
                    size: 48,
                    onInk: true,
                    onResult: onPictureResult,
                    editor: pictureEditor
                )
            } else {
                CabalMark(
                    groupId: view.id,
                    name: view.name,
                    size: 48,
                    onInk: true,
                    pictureUrl: view.pictureUrl,
                    accessibilityLabel: view.pictureUrl == nil ? nil : "\(view.name) picture"
                )
            }
            VStack(alignment: .leading, spacing: 4) {
                Text(view.name)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.onHero)
                    .lineLimit(2)
                    .minimumScaleFactor(0.75)
                    .accessibilityAddTraits(.isHeader)
            }
        }
    }

    // MARK: - The pot

    private var pot: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("In the pot")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onHeroMuted)
            MoneyText(decimalString: view.resolvedPotTotalUsd, style: .hero, color: MonacoTheme.onHero)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
                .dynamicTypeSize(...DynamicTypeSize.accessibility2)
                .accessibilityIdentifier("pot-total-value")
            HStack(spacing: MonacoTheme.Space.s) {
                PnLBadge(dollarPnl: GroupHeroMath.potDollarPnl(view.pot), percentReturn: nil, onInk: true)
                Text("all time")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onHeroMuted)
            }
        }
        .accessibilityElement(children: .combine)
    }

    // MARK: - Your slice

    private var slice: some View {
        HStack(alignment: .lastTextBaseline, spacing: MonacoTheme.Space.s) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Your slice")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onHeroMuted)
                MoneyText(decimalString: view.you.equityUsd, style: .large, color: MonacoTheme.onHero)
            }
            Spacer(minLength: MonacoTheme.Space.s)
            VStack(alignment: .trailing, spacing: 2) {
                Text(GroupHeroMath.sliceCaption(view.you))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onHeroMuted)
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
                if GroupHeroMath.hasSlice(view.you) {
                    PnLText(dollarPnl: view.you.dollarPnl, style: .row, onInk: true)
                }
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("group-hero-slice")
    }
}

/// Figures the hero derives from the view DTO. Parses raw server strings, never formatted ones.
enum GroupHeroMath {
    /// Pot P&L = sum of every holding's dollar P&L, as a signed server-style string ("+50.58").
    static func potDollarPnl(_ pot: [PotRowDTO]) -> String {
        let total = pot.reduce(Decimal.zero) { $0 + (decimal(from: $1.dollarPnl) ?? .zero) }
        var value = total
        var rounded = Decimal()
        NSDecimalRound(&rounded, &value, 2, .plain)
        let magnitude = pnlFormatter.string(from: NSDecimalNumber(decimal: rounded < 0 ? -rounded : rounded)) ?? "0.00"
        return (rounded < 0 ? "-" : "+") + magnitude
    }

    /// Built once: the hero recomputes this on every body pass.
    private static let pnlFormatter: NumberFormatter = {
        let formatter = NumberFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.minimumFractionDigits = 2
        formatter.maximumFractionDigits = 2
        formatter.minimumIntegerDigits = 1
        return formatter
    }()

    static func hasSlice(_ slice: MemberSliceDTO) -> Bool {
        (Double(slice.slicePercent) ?? 0) > 0
    }

    /// "57% of the pot", or a nudge when the member hasn't put money in yet.
    static func sliceCaption(_ slice: MemberSliceDTO) -> String {
        guard let fraction = Double(slice.slicePercent), fraction > 0 else {
            return "Fund to get a slice"
        }
        let percent = fraction * 100
        let label =
            percent >= 10 || percent.rounded() == percent
            ? String(format: "%.0f%%", percent)
            : String(format: "%.1f%%", percent)
        return "\(label) of the pot"
    }

    static func decimal(from raw: String) -> Decimal? {
        var trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: "\u{2212}", with: "-")
            .replacingOccurrences(of: "$", with: "")
            .replacingOccurrences(of: ",", with: "")
        if trimmed.hasPrefix("+") { trimmed.removeFirst() }
        return Decimal(string: trimmed, locale: Locale(identifier: "en_US_POSIX"))
    }
}
