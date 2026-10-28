import SwiftUI

struct MonacoToast: Equatable, Identifiable {
    let id = UUID()
    let message: String
    var isSuccess = false
    var link: MonacoToastLink?
    var action: MonacoToastAction?
}

struct MonacoToastLink: Equatable {
    let title: String
    let url: URL
}

/// A button on a toast, such as Undo.
///
/// The server owns the window an action is good for, not the toast: the caller hides the action by
/// not passing one, and passes `expiresAt` so the button goes when the window closes. A late tap
/// is refused by the endpoint behind it, and the screen that passed the action shows its own error
/// toast then.
struct MonacoToastAction: Equatable {
    let title: String
    var expiresAt: Date?
    let perform: @MainActor () -> Void

    func isAvailable(at now: Date) -> Bool {
        expiresAt.map { now < $0 } ?? true
    }

    static func == (lhs: MonacoToastAction, rhs: MonacoToastAction) -> Bool {
        lhs.title == rhs.title
    }
}

/// F's toast: a capsule of system material, with a glyph, one or two short sentences and an
/// optional action.
///
/// The toast has its own `toast*` tokens. It used to borrow `primaryButton*`, so when the brand
/// accent became electric blue the toast turned into a blue capsule: the green success glyph fell
/// to 1.3:1 on it, and an error toast read as a second primary button above the real one.
struct MonacoToastBanner: View {
    let message: String
    var isSuccess = false
    var link: MonacoToastLink?
    var action: MonacoToastAction?
    var insetsGutter = true
    /// Draws the Reduce Transparency fill without the setting, which the environment only reads.
    var opaque = false
    var focus: AccessibilityFocusState<Bool>.Binding?
    var onDismiss: () -> Void = {}

    @ScaledMetric(relativeTo: .subheadline) private var glyphSize: CGFloat = MonacoToastLayout.glyphSize
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    @Environment(\.displayScale) private var displayScale
    @State private var gate = MonacoToastActionGate()

    var body: some View {
        HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
            // Filled circle, not a bare tick: the state still reads without relying on hue.
            Image(systemName: isSuccess ? "checkmark.circle.fill" : "exclamationmark.circle.fill")
                .font(.system(size: glyphSize, weight: .semibold))
                .foregroundStyle(isSuccess ? MonacoTheme.toastSuccessGlyph : MonacoTheme.toastErrorGlyph)
                .accessibilityHidden(true)
            Text(message)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.toastLabel)
                .multilineTextAlignment(.leading)
                .lineLimit(2)
                .fixedSize(horizontal: false, vertical: true)
                .toastFocus(focus)
            if let link {
                Link(link.title, destination: link.url)
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.tint)
                    .frame(minWidth: MonacoToastLayout.minimumHeight, minHeight: MonacoToastLayout.minimumHeight)
                    .toastFocus(focus)
                    .accessibilityIdentifier("monaco-toast-link")
            }
            if let action {
                Button(action.title) {
                    guard gate.run(perform: action.perform, dismiss: onDismiss) else { return }
                    Haptics.selection()
                }
                .buttonStyle(.plain)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.tint)
                .frame(minWidth: MonacoToastLayout.minimumHeight, minHeight: MonacoToastLayout.minimumHeight)
                .contentShape(Rectangle())
                .toastFocus(focus)
                .accessibilityIdentifier("monaco-toast-action")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoToastLayout.minimumHeight)
        .background { capsule }
        .shadow(MonacoTheme.Shadow.floating)
        .padding(.horizontal, insetsGutter ? MonacoTheme.Space.gutter : 0)
        .frame(maxWidth: insetsGutter ? .infinity : nil, alignment: .leading)
        .accessibilityElement(children: link == nil && action == nil ? .combine : .contain)
        .accessibilityIdentifier("monaco-toast-banner")
    }

    @ViewBuilder
    private var capsule: some View {
        if reduceTransparency || opaque {
            Capsule()
                .fill(MonacoTheme.toastFill)
                .overlay { Capsule().strokeBorder(MonacoTheme.toastStroke, lineWidth: 1 / displayScale) }
        } else {
            Capsule().fill(.regularMaterial)
        }
    }
}

extension View {
    fileprivate func shadow(_ layer: MonacoTheme.ShadowLayer) -> some View {
        shadow(color: layer.color, radius: layer.radius, x: 0, y: layer.y)
    }

    fileprivate func toastFocus(_ binding: AccessibilityFocusState<Bool>.Binding?) -> some View {
        modifier(MonacoToastFocus(binding: binding))
    }
}

private struct MonacoToastFocus: ViewModifier {
    let binding: AccessibilityFocusState<Bool>.Binding?

    @ViewBuilder
    func body(content: Content) -> some View {
        if let binding {
            content.accessibilityFocused(binding)
        } else {
            content
        }
    }
}

enum MonacoToastLayout {
    /// The toast sits in the navigation bar's row, over the title.
    static let alignment: Alignment = .top

    static let minimumHeight: CGFloat = 44
    static let glyphSize: CGFloat = 17
    /// How far the capsule slides in from above.
    static let slide: CGFloat = MonacoTheme.Space.s
}

/// What takes a toast down: its action, a swipe up and its countdown. A tap on the rest of the
/// capsule does not, so a near-miss on Undo keeps the toast.
enum MonacoToastDismissal {
    static let swipeUpDistance: CGFloat = 24
    static let predictedSwipeUpDistance: CGFloat = 60
    static let tapOutsideTheActionDismisses = false

    static func swipeDismisses(translation: CGFloat, predicted: CGFloat) -> Bool {
        translation < -swipeUpDistance || predicted < -predictedSwipeUpDistance
    }
}

/// Lets an action run once. A second tap on Undo, before the toast is gone, does nothing.
struct MonacoToastActionGate {
    private(set) var hasRun = false

    @MainActor
    mutating func run(perform: @MainActor () -> Void, dismiss: () -> Void) -> Bool {
        guard !hasRun else { return false }
        hasRun = true
        perform()
        dismiss()
        return true
    }
}

/// How long a toast stays up.
///
/// A fixed 2.5s is too short for the money-flow failures that go through this path — for example
/// "We couldn't confirm that went through. Check your balance before trying again." — and there is
/// nowhere to read them again.
enum MonacoToastTiming {
    /// Rough reading speed, in seconds per character.
    static let secondsPerCharacter: TimeInterval = 0.06

    static let base: TimeInterval = 2
    static let successFloor: TimeInterval = 2.5
    static let failureFloor: TimeInterval = 4
    static let ceiling: TimeInterval = 10

    /// A toast with an action stays long enough to reach it.
    static let actionFloor: TimeInterval = 4

    /// VoiceOver reads the whole announcement before the user can act on it.
    static let voiceOverFactor: Double = 1.5
    static let voiceOverCeiling: TimeInterval = 15

    /// The longest a drag may hold the countdown open past the toast's own dwell.
    ///
    /// The hold is driven by a gesture flag, and SwiftUI does not deliver `onEnded` when a gesture
    /// is cancelled rather than completed — the banner torn down mid-drag, the screen pushed away,
    /// a system edge gesture winning. Without a ceiling the flag stays set, the countdown never
    /// decrements, and the toast pins itself over a money screen with tap as the only way out.
    static let maximumHold: TimeInterval = 2.5

    /// The toast outlives its action's `expiresAt` on screen, but the button goes at the expiry.
    /// Under VoiceOver a toast with an action stays until `expiresAt`, capped at `voiceOverCeiling`.
    static func dwell(
        message: String, isSuccess: Bool, voiceOverRunning: Bool,
        action: MonacoToastAction? = nil, now: Date = Date()
    ) -> TimeInterval {
        let reading = base + secondsPerCharacter * Double(message.count)
        var floor = isSuccess ? successFloor : failureFloor
        if action != nil { floor = max(floor, actionFloor) }
        let dwell = min(max(reading, floor), ceiling)
        guard voiceOverRunning else { return dwell }
        guard let action else { return min(dwell * voiceOverFactor, voiceOverCeiling) }
        let window = action.expiresAt.map { $0.timeIntervalSince(now) } ?? dwell * voiceOverFactor
        return min(max(window, dwell), voiceOverCeiling)
    }
}

/// The dwell countdown, as state rather than as a loop, so the hold and its ceiling are testable
/// without a view or a running clock.
///
/// A drag pauses the countdown so a toast is not dismissed out from under a moving thumb, but the
/// pause is bounded by wall clock: `elapsed` keeps running while `isHeld` is true, and once the
/// toast has been up for `dwell + maximumHold` it goes regardless. That bound is what a flag alone
/// cannot give — see `MonacoToastTiming.maximumHold`.
struct MonacoToastCountdown {
    private(set) var remaining: TimeInterval
    private let limit: TimeInterval
    private var elapsed: TimeInterval = 0

    init(dwell: TimeInterval, maximumHold: TimeInterval = MonacoToastTiming.maximumHold) {
        remaining = dwell
        limit = dwell + maximumHold
    }

    /// True once the toast should come down: either the countdown ran out or the hold ceiling did.
    var isFinished: Bool { remaining <= 0 || elapsed >= limit }

    /// Advances by one slice. A held toast keeps its `remaining` but still spends wall clock.
    mutating func tick(slice: TimeInterval, isHeld: Bool) {
        elapsed += slice
        guard !isHeld else { return }
        remaining -= slice
    }
}

private struct MonacoToastModifier: ViewModifier {
    @Binding var toast: MonacoToast?
    let now: () -> Date

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.accessibilityVoiceOverEnabled) private var voiceOverEnabled
    @State private var dragOffset: CGFloat = 0
    @State private var isDragging = false
    @State private var actionExpired = false
    @AccessibilityFocusState private var bannerFocused: Bool

    private var transitionAnimation: Animation {
        .easeOut(duration: 0.25)
    }

    func body(content: Content) -> some View {
        content
            .overlay(alignment: MonacoToastLayout.alignment) {
                // The animation lives on this container, never on `content`: a toast set in the
                // same update as a balance or a list reload used to spring the whole screen.
                ZStack(alignment: MonacoToastLayout.alignment) {
                    if let toast {
                        banner(toast)
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: MonacoToastLayout.alignment)
                .animation(transitionAnimation, value: toast?.id)
            }
            .onChange(of: toast?.id, initial: true) { _, newID in
                guard newID != nil, let current = toast else { return }
                dragOffset = 0
                isDragging = false
                if !current.isSuccess {
                    Haptics.warning()
                }
                AccessibilityNotification.Announcement(announcement(for: current)).post()
            }
            .task(id: toast?.id) {
                guard let current = toast else { return }
                // A drag that was cancelled rather than ended leaves this set; a new toast must not
                // inherit it, or it starts its life already held.
                isDragging = false
                actionExpired = false
                refreshAction(of: current)
                await dismiss(
                    current,
                    after: MonacoToastTiming.dwell(
                        message: current.message,
                        isSuccess: current.isSuccess,
                        voiceOverRunning: voiceOverEnabled,
                        action: current.action,
                        now: now()
                    ))
            }
            .onDisappear { isDragging = false }
    }

    private func announcement(for toast: MonacoToast) -> String {
        guard let action = toast.action, action.isAvailable(at: now()) else { return toast.message }
        return "\(toast.message). \(action.title) available."
    }

    private func banner(_ toast: MonacoToast) -> some View {
        HStack(spacing: 0) {
            MonacoToastBanner(
                message: toast.message, isSuccess: toast.isSuccess, link: toast.link,
                action: actionExpired ? nil : toast.action, insetsGutter: false,
                focus: $bannerFocused, onDismiss: { self.toast = nil }
            )
            .gesture(
                DragGesture(minimumDistance: 8)
                    .onChanged {
                        isDragging = true
                        dragOffset = $0.translation.height
                    }
                    .onEnded { value in
                        isDragging = false
                        if MonacoToastDismissal.swipeDismisses(
                            translation: value.translation.height, predicted: value.predictedEndTranslation.height)
                        {
                            self.toast = nil
                        }
                        withAnimation(.snappy) { dragOffset = 0 }
                    }
            )
            Spacer(minLength: 0)
        }
        .id(toast.id)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .offset(y: min(dragOffset, 0))
        .transition(
            reduceMotion
                ? .opacity
                : .offset(y: -MonacoToastLayout.slide).combined(with: .opacity)
        )
        .zIndex(1)
    }

    private func refreshAction(of toast: MonacoToast) {
        guard let action = toast.action, !actionExpired, !action.isAvailable(at: now()) else { return }
        actionExpired = true
    }

    /// Counts down in slices so a toast is not dismissed out from under a dragging thumb, and never
    /// past `MonacoToastTiming.maximumHold` beyond its dwell even if the drag never reports an end.
    /// A toast with an action does not count down while VoiceOver focus is on it.
    private func dismiss(_ current: MonacoToast, after dwell: TimeInterval) async {
        let slice: TimeInterval = 0.1
        var countdown = MonacoToastCountdown(dwell: dwell)
        while !countdown.isFinished {
            do {
                try await Task.sleep(for: .seconds(slice))
            } catch {
                return
            }
            refreshAction(of: current)
            if voiceOverEnabled, current.action != nil, bannerFocused { continue }
            countdown.tick(slice: slice, isHeld: isDragging)
        }
        if toast?.id == current.id {
            toast = nil
        }
    }
}

extension View {
    /// Nav-row toast: swipe up to dismiss, otherwise it dwells for as long as its message takes to
    /// read, and at least `MonacoToastTiming.actionFloor` with an action. A tap on the capsule does
    /// not dismiss it; its action does.
    ///
    /// A drag in progress pauses the countdown, so a toast is not pulled out from under a moving
    /// thumb. A finger resting on the toast without moving does not pause it: the dismiss gesture
    /// only begins after 8pt of travel, so there is nothing to tell a still finger from no finger.
    /// `now` is the clock an action's `expiresAt` is read against.
    func monacoToast(_ toast: Binding<MonacoToast?>, now: @escaping () -> Date = Date.init) -> some View {
        modifier(MonacoToastModifier(toast: toast, now: now))
    }
}
