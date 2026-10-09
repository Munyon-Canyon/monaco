import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalsListSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        MyCabalsList()
    }
}

private struct MyCabalsList: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: MonacoCore.CabalsTabModel?
    @State private var showsNewCabal = false

    var body: some View {
        MyCabalsContent(model: model, standings: model?.standings ?? [:], open: open, create: { showsNewCabal = true })
            .task {
                let model = preparedModel()
                refresh?.register("cabals-list") {
                    await model.load()
                    await model.loadStandings()
                }
                await model.load()
                await model.loadStandings()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
            .task { await preparedModel().observe(hints: environment.hints) }
            .newCabalSheet(isPresented: $showsNewCabal)
            .onChange(of: model?.failureTick) { _, _ in
                guard case .loaded = model?.state, let error = model?.lastError else { return }
                toasts.show(error)
            }
    }

    private func preparedModel() -> MonacoCore.CabalsTabModel {
        if let model { return model }
        let created = MonacoCore.CabalsTabModel(api: environment.api)
        model = created
        return created
    }

    private func open(_ cabal: Components.Schemas.MyCabal) {
        Haptics.selection()
        environment.navigator.open(CabalRoute(id: cabal.id), in: .cabals)
    }
}

struct MyCabalsContent: View {
    let model: MonacoCore.CabalsTabModel?
    let standings: [String: PortfolioSummary.Row]
    let open: (Components.Schemas.MyCabal) -> Void
    let create: () -> Void

    @Environment(\.accountRestricted) private var accountRestricted
    @ScaledMetric(relativeTo: .body) private var cardWidth: CGFloat = 168

    static let cardHeight: CGFloat = 128
    private var cardSize: CGSize { CGSize(width: cardWidth, height: Self.cardHeight) }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Your cabals")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            content
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-list")
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            placeholders
        case .failed:
            MonacoErrorRow(thing: "your cabals", identifier: "cabals-list-failed") {
                Task { await model?.load() }
            }
        case .loaded(let cabals) where cabals.isEmpty:
            EmptyState(
                title: "No cabals yet",
                message: accountRestricted
                    ? "Search above to find one." : "Start one with friends, or search above to find one.",
                actionTitle: accountRestricted ? nil : "Start a cabal",
                action: accountRestricted ? nil : create
            )
            .accessibilityIdentifier("cabals-list-empty")
        case .loaded(let cabals):
            cards(cabals)
        }
    }

    private func cards(_ cabals: [Components.Schemas.MyCabal]) -> some View {
        ScrollView(.horizontal, showsIndicators: false) {
            LazyHStack(spacing: MonacoTheme.Space.s) {
                ForEach(cabals, id: \.id) { cabal in
                    Button {
                        open(cabal)
                    } label: {
                        MyCabalCard(cabal: cabal, standing: standings[cabal.id], size: cardSize)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("cabals-list-card-\(cabal.id)")
                }
                if !accountRestricted {
                    Button(action: create) {
                        NewCabalCard(size: cardSize)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("cabals-list-new")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.xs)
        }
    }

    private var placeholders: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            ForEach(0..<2, id: \.self) { _ in
                SkeletonBlock(
                    width: cardSize.width, height: cardSize.height, radius: MonacoTheme.Radius.card)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement()
        .accessibilityLabel("Loading your cabals")
        .accessibilityIdentifier("cabals-list-loading")
    }
}

private struct MyCabalCard: View {
    let cabal: Components.Schemas.MyCabal
    let standing: PortfolioSummary.Row?
    let size: CGSize

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var tint: MonacoTheme.CabalTint { .forGroupId(cabal.id) }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            HStack(alignment: .top) {
                CabalMark(groupId: cabal.id, name: cabal.name, size: 36, pictureUrl: cabal.pictureUrl)
                Spacer(minLength: 0)
                if let unread = CabalCopy.unreadBadge(cabal.unreadCount) {
                    Text(unread)
                        .font(MonacoTheme.Typo.caption.weight(.semibold).monospacedDigit())
                        .foregroundStyle(MonacoTheme.onDestructive)
                        .padding(.horizontal, MonacoTheme.Space.s)
                        .frame(minWidth: 22, minHeight: 22)
                        .background(Capsule().fill(MonacoTheme.destructive))
                        .accessibilityLabel(CabalCopy.unreadLabel(cabal.unreadCount))
                        .accessibilityIdentifier("cabals-list-card-unread")
                }
                if let requests = CabalCopy.requestBadge(cabal) {
                    Text("\(requests)")
                        .font(MonacoTheme.Typo.caption.weight(.semibold).monospacedDigit())
                        .foregroundStyle(MonacoTheme.onBrand)
                        .padding(.horizontal, MonacoTheme.Space.s)
                        .frame(minWidth: 22, minHeight: 22)
                        .background(Capsule().fill(MonacoTheme.brandFill))
                        .accessibilityLabel(CabalCopy.requestCount(requests))
                        .accessibilityIdentifier("cabals-list-card-requests")
                }
            }
            Text(cabal.name)
                .font(MonacoTheme.Typo.rowTitle)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 2)
                .multilineTextAlignment(.leading)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, MonacoTheme.Space.xs)
            if let standing {
                MoneyText(micros: standing.valueMicros, style: .row)
                    .accessibilityIdentifier("cabals-list-card-pot")
                if let bps = standing.returnBps {
                    PnLBadge(signedMicros: standing.pnlMicros, basisPoints: bps)
                        .accessibilityIdentifier("cabals-list-card-return")
                }
            }
            Spacer(minLength: 0)
        }
        .padding(MonacoTheme.Space.m)
        .frame(width: size.width, alignment: .topLeading)
        .frame(minHeight: size.height, alignment: .topLeading)
        .background(tint.soft, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .strokeBorder(tint.fill.opacity(0.22), lineWidth: 1)
        }
        .accessibilityElement(children: .combine)
    }
}

private struct NewCabalCard: View {
    let size: CGSize

    var body: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            Image(systemName: "plus")
                .font(.title2.weight(.semibold))
                .foregroundStyle(MonacoTheme.muted)
            Text("New cabal")
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.muted)
        }
        .frame(width: size.width, height: size.height)
        .overlay {
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .strokeBorder(MonacoTheme.hairline, style: StrokeStyle(lineWidth: 1, dash: [6, 4]))
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("New cabal")
    }
}
