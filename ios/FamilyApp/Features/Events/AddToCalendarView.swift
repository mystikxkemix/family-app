import EventKit
import EventKitUI
import SwiftUI

/// Apple's "new event" screen, filled with a shared event. The user picks the
/// calendar and taps Add. Since iOS 17 this screen needs no calendar permission:
/// it runs outside the app, and the app never reads the calendar.
struct AddToCalendarView: UIViewControllerRepresentable {
    let event: FamilyEvent
    @Environment(\.dismiss) private var dismiss

    func makeCoordinator() -> Coordinator {
        Coordinator(dismiss: dismiss)
    }

    func makeUIViewController(context: Context) -> EKEventEditViewController {
        let store = EKEventStore()
        let calendarEvent = EKEvent(eventStore: store)
        calendarEvent.title = event.title
        calendarEvent.isAllDay = event.allDay
        calendarEvent.startDate = event.startsAt
        // Without an end, a timed event lasts one hour and an all-day event one day.
        calendarEvent.endDate = event.endsAt ?? event.startsAt.addingTimeInterval(event.allDay ? 0 : 3600)
        calendarEvent.location = event.location.isEmpty ? nil : event.location
        calendarEvent.notes = event.notes.isEmpty ? nil : event.notes

        let controller = EKEventEditViewController()
        controller.eventStore = store
        controller.event = calendarEvent
        controller.editViewDelegate = context.coordinator
        return controller
    }

    func updateUIViewController(_ controller: EKEventEditViewController, context: Context) {}

    // @preconcurrency: EventKitUI is older than Swift concurrency, but calls
    // its delegate on the main thread like the rest of UIKit.
    @MainActor
    final class Coordinator: NSObject, @preconcurrency EKEventEditViewDelegate {
        private let dismiss: DismissAction

        init(dismiss: DismissAction) {
            self.dismiss = dismiss
        }

        func eventEditViewController(_ controller: EKEventEditViewController, didCompleteWith action: EKEventEditViewAction) {
            dismiss()
        }
    }
}
