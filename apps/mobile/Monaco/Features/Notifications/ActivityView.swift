import MonacoCore
import SwiftUI

struct ActivityView: View {
    @State private var model: ActivityFeedModel?

    init(model: ActivityFeedModel? = nil) {
        _model = State(initialValue: model)
    }

    var body: some View {
        ActivityContent(model: model)
            .task {
                let model = preparedModel()
                await model.load()
            }
    }

    private func preparedModel() -> ActivityFeedModel {
        if let model { return model }
        let created = ActivityFeedModel()
        model = created
        return created
    }
}

struct ActivityContent: View {
    let model: ActivityFeedModel?

    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        ScrollView {
            content
        }
        .monacoCanvas()
        .navigationTitle(ActivityFeedCopy.title)
        .navigationBarTitleDisplayMode(.inline)
    }

    @ViewBuilder private var content: some View {
        switch model?.phase ?? .loading {
        case .loading:
            MonacoRowSkeleton(rows: 5, markShape: .circle)
                .padding(.top, MonacoTheme.Space.s)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(ActivityFeedCopy.loadingLabel)
                .accessibilityIdentifier("activity-loading")
        case .failed:
            ScreenLoadError(identifier: "activity-error") { Task { await model?.load() } }
        case .empty:
            VStack(spacing: 0) {
                markAllRead
                EmptyState(
                    title: ActivityFeedCopy.emptyTitle, message: ActivityFeedCopy.emptyMessage,
                    systemImage: MonacoIcon.bell.symbol
                )
                .accessibilityIdentifier("activity-empty")
            }
        case .loaded:
            if let model { loaded(model) }
        case .comingSoon:
            Text(ActivityFeedCopy.comingSoon)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .multilineTextAlignment(.center)
                .frame(maxWidth: .infinity)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.section)
                .accessibilityIdentifier("activity-coming-soon")
        }
    }

    private var markAllRead: some View {
        HStack {
            Spacer(minLength: 0)
            Button(ActivityFeedCopy.markAllRead) {
                withAnimation(.easeOut(duration: 0.2)) { model?.markAllRead() }
            }
            .buttonStyle(.monacoText)
            .accessibilityIdentifier("activity-mark-all-read")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
    }

    private func loaded(_ model: ActivityFeedModel) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            markAllRead
            Text(model.groupLabel)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.xs)
                .padding(.bottom, MonacoTheme.Space.sm)
                .accessibilityAddTraits(.isHeader)
            ForEach(model.items) { item in
                row(item)
            }
        }
    }

    @ViewBuilder private func row(_ item: ActivityFeedItem) -> some View {
        switch item.kind {
        case .accountUpdate:
            accountUpdate(item)
        case .reply, .vote, .proposalPassed, .needsVote:
            social(item)
        }
    }

    private func accountUpdate(_ item: ActivityFeedItem) -> some View {
        Button {
            open(item)
        } label: {
            MonacoRow(title: item.title, subtitle: item.metaLine, chevron: true) {
                SunkenGlyphMark(icon: .wallet)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("activity-item-\(item.id)")
    }

    private func social(_ item: ActivityFeedItem) -> some View {
        Button {
            open(item)
        } label: {
            HStack(spacing: MonacoTheme.Space.sm) {
                HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                    MonacoAvatar(
                        photoURL: item.actorPhotoURL, displayName: item.actorName ?? "", size: 38, seed: item.actorID)
                    texts(item)
                }
                MonacoRowChevron()
            }
            .padding(.vertical, MonacoTheme.Space.m)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .background {
                if !item.isRead {
                    RoundedRectangle(cornerRadius: MonacoTheme.Radius.cardTight, style: .continuous)
                        .fill(MonacoTheme.brandSoft)
                }
            }
            .overlay(alignment: .bottom) {
                if item.isRead { MonacoRule(color: MonacoTheme.hairline) }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, item.isRead ? 0 : MonacoTheme.Space.s)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(item.spokenText)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("activity-item-\(item.id)")
    }

    private func texts(_ item: ActivityFeedItem) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(item.title)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.leading)
            Text(item.body)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.bodyText)
                .lineLimit(3)
                .multilineTextAlignment(.leading)
            Text(item.metaLine)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .multilineTextAlignment(.leading)
            if let badge = item.badge {
                StatusBadge(badge.text, tone: tone(badge.tone))
                    .padding(.top, MonacoTheme.Space.xs)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func tone(_ tone: ActivityFeedBadge.Tone) -> StatusBadge.Tone {
        switch tone {
        case .warn: .warn
        case .positive: .positive
        case .neutral: .neutral
        }
    }

    private func open(_ item: ActivityFeedItem) {
        model?.markRead(id: item.id)
        switch item.kind {
        case .accountUpdate:
            environment.navigator.open(AccountActivityRoute(), in: .home)
        case .reply, .vote, .proposalPassed, .needsVote:
            PushRouter(environment: environment).handle(item.destination)
        }
    }
}

#if DEBUG
final class ActivityFeedSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-activityFeedHarness") else { return nil }
        let name = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "unread"
        let scenario = ActivityFeedModel.PreviewScenario(rawValue: name) ?? .unread
        return AnyView(
            NavigationStack {
                ActivityView(model: .preview(scenario))
                    .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
            }
        )
    }
}
#endif
