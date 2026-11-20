import SwiftUI

/// Restaurants, bars and activities to try together.
struct PlacesView: View {
    @Environment(AppSession.self) private var session
    @Environment(\.openURL) private var openURL
    @State private var places: [Place] = []
    @State private var status = PlaceStatus.toTry
    @State private var hasLoaded = false
    @State private var editing: PlaceEditor.Target?

    private var visiblePlaces: [Place] {
        let filtered = places.filter { $0.status == status }
        // Tried places: best rated first.
        return status == .tried ? filtered.sorted { $0.rating > $1.rating } : filtered
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Picker("Status", selection: $status) {
                        ForEach(PlaceStatus.allCases) { status in
                            Text(status.title).tag(status)
                        }
                    }
                    .pickerStyle(.segmented)
                    .accessibilityIdentifier("status.picker")
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }

                ForEach(visiblePlaces) { place in
                    PlaceRow(place: place)
                        .contentShape(Rectangle())
                        .onTapGesture { editing = .edit(place) }
                        .swipeActions(edge: .leading) {
                            if place.status == .toTry {
                                Button("Tried", systemImage: "checkmark") {
                                    update(place, ["status": .string(PlaceStatus.tried.rawValue)])
                                }
                                .tint(.green)
                            } else {
                                Button("To try", systemImage: "arrow.uturn.backward") {
                                    update(place, ["status": .string(PlaceStatus.toTry.rawValue)])
                                }
                                .tint(.orange)
                            }
                        }
                        .swipeActions(edge: .trailing) {
                            Button("Delete", systemImage: "trash", role: .destructive) { delete(place) }
                            interestButton(place)
                        }
                        .contextMenu { contextMenu(place) }
                }
            }
            .navigationTitle("Places")
            .overlay {
                if hasLoaded && visiblePlaces.isEmpty {
                    ContentUnavailableView {
                        Label("No places yet", systemImage: "fork.knife")
                    } description: {
                        Text("Add a restaurant, a bar or an activity you want to try together.")
                    }
                }
            }
            .toolbar {
                Button("Add", systemImage: "plus") { editing = .create }
                    .accessibilityIdentifier("toolbar.add")
            }
            .sheet(item: $editing) { target in
                PlaceEditor(target: target) { await load() }
            }
            .refreshable { await load() }
            .task { await load() }
            .onFamilyDataChanged { await load() }
        }
    }

    @ViewBuilder
    private func contextMenu(_ place: Place) -> some View {
        if let mapsURL = mapsURL(place) {
            Button("Open in Maps", systemImage: "map") { openURL(mapsURL) }
        }
        if let website = URL(string: place.url), !place.url.isEmpty {
            Button("Open website", systemImage: "safari") { openURL(website) }
        }
        if place.status == .tried {
            Menu("Rate", systemImage: "star") {
                ForEach((1...5).reversed(), id: \.self) { stars in
                    Button(String(repeating: "★", count: stars)) { update(place, ["rating": .int(stars)]) }
                }
            }
        }
    }

    private func interestButton(_ place: Place) -> some View {
        let interested = place.interestedUserIds.contains(session.me?.user.id ?? "")
        return Button(
            interested ? LocalizedStringKey("Not for me") : LocalizedStringKey("I want to go"),
            systemImage: interested ? "heart.slash" : "heart"
        ) {
            update(place, ["interested": .bool(!interested)])
        }
        .tint(.pink)
    }

    /// Apple Maps search for the name and the address.
    private func mapsURL(_ place: Place) -> URL? {
        var components = URLComponents(string: "https://maps.apple.com/")
        components?.queryItems = [URLQueryItem(name: "q", value: [place.name, place.address].filter { !$0.isEmpty }.joined(separator: ", "))]
        return components?.url
    }

    private func load() async {
        await session.run { api in
            places = try await api.get("places")
            hasLoaded = true
        }
    }

    private func update(_ place: Place, _ body: JSONBody) {
        Task {
            await session.run { api in
                let updated: Place = try await api.patch("places/\(place.id)", body)
                if let index = places.firstIndex(where: { $0.id == updated.id }) {
                    withAnimation { places[index] = updated }
                }
            }
        }
    }

    private func delete(_ place: Place) {
        places.removeAll { $0.id == place.id }
        Task { await session.run { api in try await api.delete("places/\(place.id)") } }
    }
}

private struct PlaceRow: View {
    let place: Place
    @Environment(AppSession.self) private var session

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: place.category.symbol)
                .font(.title2)
                .foregroundStyle(.tint)
                .frame(width: 32)
            VStack(alignment: .leading, spacing: 2) {
                Text(place.name).font(.headline)
                if !place.address.isEmpty {
                    Text(place.address)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                if !place.note.isEmpty {
                    Text(place.note)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
                if place.status == .tried && place.rating > 0 {
                    Text(String(repeating: "★", count: place.rating) + String(repeating: "☆", count: 5 - place.rating))
                        .font(.caption)
                        .foregroundStyle(.orange)
                } else {
                    InterestLine(userIds: place.interestedUserIds)
                }
            }
        }
        .padding(.vertical, 4)
    }
}

/// Who wants to watch or to go, with a special line when it is everyone.
struct InterestLine: View {
    let userIds: [String]
    @Environment(AppSession.self) private var session

    var body: some View {
        let members = session.me?.members ?? []
        if members.count > 1 && userIds.count >= members.count {
            Label("Everyone wants it", systemImage: "heart.fill")
                .font(.caption)
                .foregroundStyle(.pink)
        } else if !userIds.isEmpty {
            Label(userIds.map { session.memberName(id: $0) }.formatted(.list(type: .and)), systemImage: "heart")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }
}

/// Adds or edits a place.
struct PlaceEditor: View {
    enum Target: Identifiable {
        case create
        case edit(Place)

        var id: String {
            switch self {
            case .create: "new"
            case .edit(let place): place.id
            }
        }
    }

    let target: Target
    let onSave: () async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var category = PlaceCategory.restaurant
    @State private var address = ""
    @State private var website = ""
    @State private var note = ""
    @State private var isSaving = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Name", text: $name)
                        .accessibilityIdentifier("editor.name")
                    Picker("Category", selection: $category) {
                        ForEach(PlaceCategory.allCases) { category in
                            Label { Text(category.title) } icon: { Image(systemName: category.symbol) }.tag(category)
                        }
                    }
                }
                Section {
                    TextField("Address or area", text: $address)
                        .textContentType(.fullStreetAddress)
                    TextField("Website (optional)", text: $website)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                }
                Section {
                    TextField("Note (who told us, what to order...)", text: $note, axis: .vertical)
                        .lineLimit(2...6)
                }
            }
            .navigationTitle(isNew ? "New place" : "Edit place")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save", action: save)
                        .accessibilityIdentifier("editor.save")
                        .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty || isSaving)
                }
            }
            .onAppear {
                if case .edit(let place) = target {
                    name = place.name
                    category = place.category
                    address = place.address
                    website = place.url
                    note = place.note
                }
            }
        }
    }

    private var isNew: Bool {
        if case .create = target { return true }
        return false
    }

    private func save() {
        isSaving = true
        var link = website.trimmingCharacters(in: .whitespaces)
        if !link.isEmpty && !link.contains("://") { link = "https://" + link }
        let body: JSONBody = [
            "name": .string(name),
            "category": .string(category.rawValue),
            "address": .string(address),
            "url": .string(link),
            "note": .string(note),
        ]
        Task {
            let saved = await session.run { api in
                switch target {
                case .create:
                    let _: Place = try await api.post("places", body)
                case .edit(let place):
                    let _: Place = try await api.patch("places/\(place.id)", body)
                }
            }
            isSaving = false
            if saved {
                await onSave()
                dismiss()
            }
        }
    }
}
