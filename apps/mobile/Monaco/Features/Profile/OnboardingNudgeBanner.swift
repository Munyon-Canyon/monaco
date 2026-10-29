import MonacoCore
import SwiftUI

struct OnboardingNudgeBanner: View {
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts
    @State private var opened: OnboardingNudge?
    @State private var pushesPhone = false

    static func nudge(profile: SessionProfile?, dismissed: Bool, connectX: Bool) -> OnboardingNudge? {
        guard !dismissed, let profile else { return nil }
        return MonacoCore.nudge(for: profile, connectX: connectX)
    }

    private var current: OnboardingNudge? {
        Self.nudge(profile: session.profile, dismissed: session.nudgeDismissed, connectX: AppFeatures.current.connectX)
    }

    private var isOpen: Binding<Bool> {
        Binding(get: { opened != nil }, set: { if !$0 { opened = nil } })
    }

    init() {
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains("-MonacoNudgeSheetOpen") {
            _opened = State(initialValue: .linkX(""))
        }
        #endif
    }

    var body: some View {
        content
            .sheet(isPresented: isOpen) {
                NavigationStack { SocialsStepView(mode: .sheet) }
                    .monacoSheet(title: LinkCopy.xTitle, detents: .votes, showsClose: true)
                    .monacoToastCenter(toasts)
            }
            .navigationDestination(isPresented: $pushesPhone) {
                PhoneStepView(mode: .sheet)
                    .safeAreaInset(edge: .top, spacing: 0) {
                        Text(LinkCopy.phoneTitle)
                            .font(MonacoTheme.Typo.display)
                            .foregroundStyle(MonacoTheme.ink)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(.horizontal, MonacoTheme.Space.gutter)
                            .accessibilityAddTraits(.isHeader)
                    }
                    .monacoCanvas()
            }
    }

    private func open(_ nudge: OnboardingNudge) {
        switch nudge {
        case .addPhone: pushesPhone = true
        case .linkX: opened = nudge
        }
    }

    @ViewBuilder
    private var content: some View {
        if let current {
            HStack(spacing: MonacoTheme.Space.s) {
                Button {
                    open(current)
                } label: {
                    MonacoNotice(.info, message: current.message)
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
                        .contentShape(Rectangle())
                }
                .accessibilityLabel("Hide")
                .accessibilityIdentifier("onboarding-nudge-close")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("onboarding-nudge")
        }
    }
}
