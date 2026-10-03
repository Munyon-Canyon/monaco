import MonacoCore
import SwiftUI

struct OnboardingNudgeBanner: View {
    @Environment(AppSessionStore.self) private var session
    @State private var opened: OnboardingNudge?

    private var current: OnboardingNudge? {
        guard !session.nudgeDismissed, let profile = session.profile else { return nil }
        return nudge(for: profile)
    }

    private var isOpen: Binding<Bool> {
        Binding(get: { opened != nil }, set: { if !$0 { opened = nil } })
    }

    var body: some View {
        if let current {
            HStack(spacing: MonacoTheme.Space.s) {
                Button {
                    opened = current
                } label: {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        Image(systemName: current.systemImage)
                            .font(MonacoTheme.Typo.bodyStrong)
                            .foregroundStyle(MonacoTheme.brandOnWash)
                            .accessibilityHidden(true)
                        Text(current.message)
                            .font(MonacoTheme.Typo.bodyStrong)
                            .foregroundStyle(MonacoTheme.ink)
                            .multilineTextAlignment(.leading)
                            .fixedSize(horizontal: false, vertical: true)
                        Spacer(minLength: 0)
                    }
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("onboarding-nudge-open")

                Button {
                    session.nudgeDismissed = true
                } label: {
                    Image(systemName: "xmark")
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.muted)
                        .frame(width: 44, height: 44)
                }
                .accessibilityLabel("Hide")
                .accessibilityIdentifier("onboarding-nudge-close")
            }
            .padding(.leading, MonacoTheme.Space.m)
            .padding(.trailing, MonacoTheme.Space.xs)
            .background(MonacoTheme.brandWash, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("onboarding-nudge")
            .sheet(isPresented: isOpen) {
                NavigationStack {
                    NotMigratedView(screen: current.screenTitle)
                        .navigationTitle(current.screenTitle)
                        .navigationBarTitleDisplayMode(.inline)
                        .toolbar {
                            ToolbarItem(placement: .confirmationAction) {
                                Button("Done") { opened = nil }
                            }
                        }
                }
                .presentationDetents([.medium])
            }
        }
    }
}

extension OnboardingNudge {
    fileprivate var systemImage: String {
        switch self {
        case .addPhone: "phone.fill"
        case .linkX: "person.2.fill"
        }
    }

    fileprivate var screenTitle: String {
        switch self {
        case .addPhone: "Add your number"
        case .linkX: "Connect X"
        }
    }
}
