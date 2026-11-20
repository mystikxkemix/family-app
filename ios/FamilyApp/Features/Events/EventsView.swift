import SwiftUI

/// Upcoming shared events, grouped by day.
struct EventsView: View {
    @Environment(AppSession.self) private var session
    @State private var events: [FamilyEvent] = []
    @State private var hasLoaded = false
    @State private var editing: EventEditor.Target?

    /// Events grouped by the day they start, soonest first.
    private var days: [(day: Date, events: [FamilyEvent])] {
        let calendar = Calendar.current
        let grouped = Dictionary(grouping: events) { calendar.startOfDay(for: $0.startsAt) }
        return grouped.keys.sorted().map { day in
            (day, grouped[day, default: []].sorted { $0.startsAt < $1.startsAt })
        }
    }

    var body: some View {
        NavigationStack {
            List {
                ForEach(days, id: \.day) { day in
                    Section {
                        ForEach(day.events) { event in
                            EventRow(event: event)
                                .contentShape(Rectangle())
                                .onTapGesture { editing = .edit(event) }
                                .swipeActions {
                                    Button("Delete", systemImage: "trash", role: .destructive) { delete(event) }
                                }
                        }
                    } header: {
                        Text(day.day, format: .dateTime.weekday(.wide).day().month(.wide))
                    }
                }
            }
            .navigationTitle("Events")
            .overlay {
                if hasLoaded && events.isEmpty {
                    ContentUnavailableView {
                        Label("No upcoming events", systemImage: "calendar")
                    } description: {
                        Text("Add a dinner, a trip, a birthday...")
                    } actions: {
                        Button("New event") { editing = .create }
                            .buttonStyle(.borderedProminent)
                    }
                }
            }
            .toolbar {
                Button("New event", systemImage: "plus") { editing = .create }
            }
            .sheet(item: $editing) { target in
                EventEditor(target: target) { await load() }
            }
            .refreshable { await load() }
            .task { await load() }
            .onFamilyDataChanged { await load() }
        }
    }

    private func load() async {
        await session.run { api in
            // From the start of today, so today's past events stay visible.
            let from = Calendar.current.startOfDay(for: .now)
            events = try await api.get("events", query: [URLQueryItem(name: "from", value: from.formatted(.iso8601))])
            hasLoaded = true
        }
    }

    private func delete(_ event: FamilyEvent) {
        events.removeAll { $0.id == event.id }
        Task { await session.run { api in try await api.delete("events/\(event.id)") } }
    }
}

private struct EventRow: View {
    let event: FamilyEvent
    @Environment(AppSession.self) private var session

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Group {
                if event.allDay {
                    Text("All day")
                } else {
                    Text(event.startsAt, format: .dateTime.hour().minute())
                }
            }
            .font(.subheadline.monospacedDigit())
            .foregroundStyle(.secondary)
            .frame(width: 64, alignment: .leading)

            VStack(alignment: .leading, spacing: 2) {
                Text(event.title).font(.headline)
                if !event.location.isEmpty {
                    Label(event.location, systemImage: "mappin.and.ellipse")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                Text("Added by \(session.memberName(id: event.createdBy))")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
            }
        }
        .padding(.vertical, 2)
    }
}

/// Creates or edits an event.
struct EventEditor: View {
    enum Target: Identifiable {
        case create
        case edit(FamilyEvent)

        var id: String {
            switch self {
            case .create: "new"
            case .edit(let event): event.id
            }
        }
    }

    let target: Target
    let onSave: () async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var title = ""
    @State private var allDay = false
    @State private var startsAt = Self.nextHour()
    @State private var hasEnd = false
    @State private var endsAt = Self.nextHour().addingTimeInterval(3600)
    @State private var location = ""
    @State private var notes = ""
    @State private var isSaving = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Title", text: $title)
                    TextField("Location", text: $location)
                }
                Section {
                    Toggle("All day", isOn: $allDay)
                    DatePicker("Starts", selection: $startsAt, displayedComponents: allDay ? .date : [.date, .hourAndMinute])
                    Toggle("End time", isOn: $hasEnd)
                    if hasEnd {
                        DatePicker("Ends", selection: $endsAt, in: startsAt..., displayedComponents: allDay ? .date : [.date, .hourAndMinute])
                    }
                }
                Section {
                    TextField("Notes", text: $notes, axis: .vertical)
                        .lineLimit(3...8)
                }
            }
            .navigationTitle(isNew ? "New event" : "Edit event")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save", action: save)
                        .disabled(title.trimmingCharacters(in: .whitespaces).isEmpty || isSaving)
                }
            }
            .onChange(of: startsAt) { _, newStart in
                if endsAt < newStart { endsAt = newStart.addingTimeInterval(3600) }
            }
            .onAppear(perform: fill)
        }
    }

    private var isNew: Bool {
        if case .create = target { return true }
        return false
    }

    private func fill() {
        guard case .edit(let event) = target else { return }
        title = event.title
        allDay = event.allDay
        startsAt = event.startsAt
        hasEnd = event.endsAt != nil
        endsAt = event.endsAt ?? event.startsAt.addingTimeInterval(3600)
        location = event.location
        notes = event.notes
    }

    private func save() {
        isSaving = true
        let calendar = Calendar.current
        // An all-day event is stored at the start of its day (local time).
        let start = allDay ? calendar.startOfDay(for: startsAt) : startsAt
        var body: JSONBody = [
            "title": .string(title),
            "startsAt": .date(start),
            "allDay": .bool(allDay),
            "location": .string(location),
            "notes": .string(notes),
        ]
        if hasEnd {
            body["endsAt"] = .date(allDay ? calendar.startOfDay(for: endsAt) : endsAt)
        } else if !isNew {
            body["clearEnd"] = .bool(true)
        }
        Task {
            let saved = await session.run { api in
                switch target {
                case .create:
                    let _: FamilyEvent = try await api.post("events", body)
                case .edit(let event):
                    let _: FamilyEvent = try await api.patch("events/\(event.id)", body)
                }
            }
            isSaving = false
            if saved {
                await onSave()
                dismiss()
            }
        }
    }

    private static func nextHour() -> Date {
        let calendar = Calendar.current
        let hour = calendar.dateInterval(of: .hour, for: .now)?.end ?? .now
        return hour
    }
}
