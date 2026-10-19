import MonacoCore
import SwiftUI

struct LeaderboardFilterSheet: View {
    let model: LeaderboardPageModel
    let dismiss: () -> Void

    var body: some View {
        @Bindable var model = model
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            field(LeaderboardCopy.sortTitle) {
                MonacoMenuSelect(
                    title: LeaderboardCopy.sortTitle, selection: $model.draftFilters.sort,
                    options: LeaderboardSort.allCases, label: \.label
                )
                .fillsWidth()
            }
            .full(model.supportsFullFilters)
            if model.category == .cabals {
                field(LeaderboardCopy.membershipTitle) {
                    MonacoMenuSelect(
                        title: LeaderboardCopy.membershipTitle, selection: $model.draftFilters.membership,
                        options: LeaderboardMembership.allCases, label: \.label
                    )
                    .fillsWidth()
                }
                .full(model.supportsFullFilters)
                field(LeaderboardCopy.minimumMembersTitle) {
                    MonacoMenuSelect(
                        title: LeaderboardCopy.minimumMembersTitle, selection: $model.draftFilters.minimumMembers,
                        options: LeaderboardMinimumMembers.allCases, label: \.label
                    )
                    .fillsWidth()
                }
                .full(model.supportsFullFilters)
            } else {
                field(LeaderboardCopy.scopeTitle) {
                    MonacoMenuSelect(
                        title: LeaderboardCopy.scopeTitle, selection: $model.draftFilters.scope,
                        options: LeaderboardPeopleScope.allCases, label: \.label
                    )
                    .fillsWidth()
                }
            }
            if !model.supportsFullFilters {
                Text(LeaderboardCopy.filtersOpenSoon)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            VStack(spacing: MonacoTheme.Space.s) {
                Button(LeaderboardCopy.apply) {
                    model.applyDraft()
                    dismiss()
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("leaderboard-filter-apply")
                Button(LeaderboardCopy.reset) {
                    model.resetFilters()
                    dismiss()
                }
                .buttonStyle(.monacoOutline)
                .accessibilityIdentifier("leaderboard-filter-reset")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .monacoFittedSheet(title: LeaderboardCopy.filterSheetTitle, showsClose: true)
    }

    private func field<Control: View>(_ title: String, @ViewBuilder control: () -> Control) -> LeaderboardField<
        Control
    > {
        LeaderboardField(title: title, control: control())
    }
}

private struct LeaderboardField<Control: View>: View {
    let title: String
    let control: Control
    var enabled = true

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(title)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.primaryText)
            control
                .disabled(!enabled)
                .opacity(enabled ? 1 : LeaderboardMetrics.disabledOpacity)
        }
    }

    func full(_ enabled: Bool) -> Self {
        var copy = self
        copy.enabled = enabled
        return copy
    }
}

struct LeaderboardAboutSheet: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            Text(LeaderboardCopy.aboutPrimary)
            Text(LeaderboardCopy.aboutSecondary)
        }
        .font(MonacoTheme.Typo.subhead)
        .foregroundStyle(MonacoTheme.secondaryText)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoFittedSheet(title: LeaderboardCopy.infoLabel, showsClose: true)
    }
}
