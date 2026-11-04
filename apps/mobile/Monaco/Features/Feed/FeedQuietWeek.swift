import MonacoAPI
import MonacoCore
import SwiftUI

struct FeedQuietWeek: View {
    let seeAll: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(spacing: 0) {
            header
            MonacoSectionHeader(
                "From everyone", trailing: "See all", actionIdentifier: "feed-see-all", action: seeAll
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    private var header: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            Image(monaco: .feed)
                .font(.largeTitle)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
            Text("Quiet week")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
            Text("Nothing new in your cabals this week. You could propose something.")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
            actions
                .padding(.top, MonacoTheme.Space.s)
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.xl)
        .padding(.bottom, MonacoTheme.Space.l)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-quiet-week")
    }

    @ViewBuilder private var actions: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(spacing: MonacoTheme.Space.sm) { buttons }
        } else {
            HStack(spacing: MonacoTheme.Space.sm) { buttons }
        }
    }

    @ViewBuilder private var buttons: some View {
        NavigationLink(value: AnyAppRoute(FriendsRoute())) {
            Label("Find people", systemImage: "person.badge.plus")
        }
        .buttonStyle(.monacoPrimary)
        .accessibilityIdentifier("feed-find-people")
        NavigationLink(value: AnyAppRoute(CreateCabalRoute())) {
            Label("Start a cabal", systemImage: MonacoIcon.plus.symbol)
        }
        .buttonStyle(.monacoSecondary)
        .accessibilityIdentifier("feed-start-cabal")
    }
}

struct FeedOutsideTally: View {
    let tally: FeedPostFacts.Tally
    let identifier: String
    var line: String?

    private var split: Text {
        Text(Image(systemName: "checkmark")).foregroundColor(MonacoTheme.profit)
            + Text(verbatim: " \(tally.yes) · ")
            + Text(Image(systemName: "xmark")).foregroundColor(MonacoTheme.loss)
            + Text(verbatim: " \(tally.no)")
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            split
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityLabel("\(tally.yes) yes, \(tally.no) no")
                .accessibilityIdentifier("feed-vote-ends-\(identifier)")
            VoteStatusLine(
                result: nil, facts: [line ?? tally.statusLine], identifierPrefix: "feed", identifierSuffix: identifier,
                minHeight: 0)
        }
        .padding(.vertical, MonacoTheme.Space.xs)
    }
}
