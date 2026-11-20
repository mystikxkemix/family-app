import SwiftUI

/// Movies and TV shows to watch together.
struct WatchlistView: View {
    @Environment(AppSession.self) private var session
    @State private var items: [Media] = []
    @State private var status = MediaStatus.toWatch
    @State private var hasLoaded = false
    @State private var editing: MediaEditor.Target?
    @State private var pick: Media?

    private var visibleItems: [Media] { items.filter { $0.status == status } }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Picker("Status", selection: $status) {
                        ForEach(MediaStatus.allCases) { status in
                            Text(status.title).tag(status)
                        }
                    }
                    .pickerStyle(.segmented)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }

                ForEach(visibleItems) { media in
                    MediaRow(media: media)
                        .contentShape(Rectangle())
                        .onTapGesture { editing = .edit(media) }
                        .swipeActions(edge: .leading) { statusActions(media) }
                        .swipeActions(edge: .trailing) {
                            Button("Delete", systemImage: "trash", role: .destructive) { delete(media) }
                            interestButton(media)
                        }
                }
            }
            .navigationTitle("Watchlist")
            .overlay {
                if hasLoaded && visibleItems.isEmpty {
                    ContentUnavailableView {
                        Label("Nothing here", systemImage: "popcorn")
                    } description: {
                        Text("Add a movie or a show you want to watch together.")
                    }
                }
            }
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Pick for us", systemImage: "dice", action: pickRandom)
                        .disabled(items.allSatisfy { $0.status != .toWatch })
                }
                ToolbarItem(placement: .primaryAction) {
                    Button("Add", systemImage: "plus") { editing = .create }
                }
            }
            .sheet(item: $editing) { target in
                MediaEditor(target: target) { await load() }
            }
            .alert(
                "Tonight's pick",
                isPresented: Binding(get: { pick != nil }, set: { if !$0 { pick = nil } }),
                presenting: pick
            ) { media in
                Button("Let's watch it") { setStatus(media, .watching) }
                Button("Pick again", action: pickRandom)
                Button("Cancel", role: .cancel) {}
            } message: { media in
                Text(media.title)
            }
            .refreshable { await load() }
            .task { await load() }
            .onFamilyDataChanged { await load() }
        }
    }

    @ViewBuilder
    private func statusActions(_ media: Media) -> some View {
        switch media.status {
        case .toWatch:
            Button("Watching", systemImage: "play") { setStatus(media, .watching) }.tint(.blue)
            Button("Watched", systemImage: "checkmark") { setStatus(media, .watched) }.tint(.green)
        case .watching:
            Button("Watched", systemImage: "checkmark") { setStatus(media, .watched) }.tint(.green)
        case .watched:
            Button("To watch", systemImage: "arrow.uturn.backward") { setStatus(media, .toWatch) }.tint(.orange)
        }
    }

    private func interestButton(_ media: Media) -> some View {
        let interested = media.interestedUserIds.contains(session.me?.user.id ?? "")
        return Button(
            interested ? LocalizedStringKey("Not for me") : LocalizedStringKey("I want to watch it"),
            systemImage: interested ? "heart.slash" : "heart"
        ) {
            update(media, ["interested": .bool(!interested)])
        }
        .tint(.pink)
    }

    private func load() async {
        await session.run { api in
            items = try await api.get("watchlist")
            hasLoaded = true
        }
    }

    private func setStatus(_ media: Media, _ newStatus: MediaStatus) {
        update(media, ["status": .string(newStatus.rawValue)])
    }

    private func update(_ media: Media, _ body: JSONBody) {
        Task {
            await session.run { api in
                let updated: Media = try await api.patch("watchlist/\(media.id)", body)
                if let index = items.firstIndex(where: { $0.id == updated.id }) {
                    withAnimation { items[index] = updated }
                }
            }
        }
    }

    private func delete(_ media: Media) {
        items.removeAll { $0.id == media.id }
        Task { await session.run { api in try await api.delete("watchlist/\(media.id)") } }
    }

    /// Picks a random title to watch. Titles that everyone wants come first.
    private func pickRandom() {
        let candidates = items.filter { $0.status == .toWatch }
        let memberCount = max(session.me?.members.count ?? 1, 1)
        let everyoneWants = candidates.filter { $0.interestedUserIds.count >= memberCount }
        pick = (everyoneWants.isEmpty ? candidates : everyoneWants).randomElement()
    }
}

private struct MediaRow: View {
    let media: Media
    @Environment(AppSession.self) private var session

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: media.kind.symbol)
                .font(.title2)
                .foregroundStyle(.tint)
                .frame(width: 32)
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline) {
                    Text(media.title).font(.headline)
                    if let year = media.year, year > 0 {
                        Text(String(year)).foregroundStyle(.secondary)
                    }
                }
                if !media.note.isEmpty {
                    Text(media.note)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
                interestLine
            }
        }
        .padding(.vertical, 4)
    }

    @ViewBuilder
    private var interestLine: some View {
        let members = session.me?.members ?? []
        let names = media.interestedUserIds.map { session.memberName(id: $0) }
        if members.count > 1 && media.interestedUserIds.count >= members.count {
            Label("Everyone wants to watch it", systemImage: "heart.fill")
                .font(.caption)
                .foregroundStyle(.pink)
        } else if !names.isEmpty {
            Label(names.formatted(.list(type: .and)), systemImage: "heart")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }
}

/// Adds or edits a movie or a show.
struct MediaEditor: View {
    enum Target: Identifiable {
        case create
        case edit(Media)

        var id: String {
            switch self {
            case .create: "new"
            case .edit(let media): media.id
            }
        }
    }

    let target: Target
    let onSave: () async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var kind = MediaKind.movie
    @State private var title = ""
    @State private var year = ""
    @State private var note = ""
    @State private var isSaving = false

    var body: some View {
        NavigationStack {
            Form {
                Picker("Type", selection: $kind) {
                    ForEach(MediaKind.allCases) { kind in
                        Label { Text(kind.title) } icon: { Image(systemName: kind.symbol) }.tag(kind)
                    }
                }
                .pickerStyle(.segmented)
                TextField("Title", text: $title)
                TextField("Year (optional)", text: $year)
                    .keyboardType(.numberPad)
                TextField("Note (who recommended it, where to watch...)", text: $note, axis: .vertical)
            }
            .navigationTitle(isNew ? "Add to watchlist" : "Edit")
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
            .onAppear {
                if case .edit(let media) = target {
                    kind = media.kind
                    title = media.title
                    year = media.year.map(String.init) ?? ""
                    note = media.note
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    private var isNew: Bool {
        if case .create = target { return true }
        return false
    }

    private func save() {
        isSaving = true
        let body: JSONBody = [
            "kind": .string(kind.rawValue),
            "title": .string(title),
            "year": .int(Int(year) ?? 0),
            "note": .string(note),
        ]
        Task {
            let saved = await session.run { api in
                switch target {
                case .create:
                    let _: Media = try await api.post("watchlist", body)
                case .edit(let media):
                    let _: Media = try await api.patch("watchlist/\(media.id)", body)
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
