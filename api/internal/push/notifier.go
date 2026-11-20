package push

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"familyapp/internal/store"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// DeviceStore is the part of the store the notifier needs.
type DeviceStore interface {
	DevicesForHousehold(ctx context.Context, householdID, exceptUserID bson.ObjectID) ([]store.Device, error)
	DeleteDeviceByToken(ctx context.Context, token string) error
}

// Sender delivers one notification to one device. *Client implements it.
type Sender interface {
	Supports(topic string) bool
	Send(ctx context.Context, deviceToken, topic, environment string, notification Notification) error
}

// Notifier tells the other members of a household that something changed.
// Sending runs in the background: a slow APNs never slows down the API.
type Notifier struct {
	devices DeviceStore
	sender  Sender // nil when push is not configured
	log     *slog.Logger
	// batchDelay is how long a batch waits for more events before it is sent.
	batchDelay time.Duration

	mu      sync.Mutex
	batches map[string]*batch
	wg      sync.WaitGroup
}

type batch struct {
	householdID bson.ObjectID
	actorID     bson.ObjectID
	count       int
	build       func(count int) Notification
	timer       *time.Timer
}

// NewNotifier returns a notifier. A nil sender turns push off: calls do nothing.
func NewNotifier(devices DeviceStore, sender Sender, log *slog.Logger) *Notifier {
	return &Notifier{
		devices:    devices,
		sender:     sender,
		log:        log,
		batchDelay: 90 * time.Second,
		batches:    make(map[string]*batch),
	}
}

// Enabled reports whether push notifications are configured.
func (n *Notifier) Enabled() bool {
	return n.sender != nil
}

// Supports reports whether a device with this bundle identifier can get pushes.
func (n *Notifier) Supports(topic string) bool {
	return n.sender != nil && n.sender.Supports(topic)
}

// Notify sends a notification to the household, except to the actor.
func (n *Notifier) Notify(householdID, actorID bson.ObjectID, notification Notification) {
	if n.sender == nil {
		return
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.send(householdID, actorID, notification)
	}()
}

// NotifyBatched groups events with the same key that happen within the batch
// delay into one notification. Adding ten groceries in a row sends one "added
// 10 items" push instead of ten. build receives the number of grouped events.
func (n *Notifier) NotifyBatched(key string, householdID, actorID bson.ObjectID, build func(count int) Notification) {
	if n.sender == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if existing, ok := n.batches[key]; ok {
		existing.count++
		existing.build = build
		return
	}
	pending := &batch{householdID: householdID, actorID: actorID, count: 1, build: build}
	n.batches[key] = pending
	n.wg.Add(1)
	pending.timer = time.AfterFunc(n.batchDelay, func() {
		defer n.wg.Done()
		if taken := n.take(key); taken != nil {
			n.send(taken.householdID, taken.actorID, taken.build(taken.count))
		}
	})
}

// Close sends the pending batches now and waits for the notifications in flight.
func (n *Notifier) Close() {
	n.mu.Lock()
	pending := make([]*batch, 0, len(n.batches))
	for key, b := range n.batches {
		pending = append(pending, b)
		delete(n.batches, key)
		// A stopped timer never runs its function, so release its wait slot here.
		if b.timer.Stop() {
			n.wg.Done()
		}
	}
	n.mu.Unlock()
	for _, b := range pending {
		n.send(b.householdID, b.actorID, b.build(b.count))
	}
	n.wg.Wait()
}

// take removes and returns the batch of a key, or nil if it was already sent.
func (n *Notifier) take(key string) *batch {
	n.mu.Lock()
	defer n.mu.Unlock()
	b := n.batches[key]
	delete(n.batches, key)
	return b
}

func (n *Notifier) send(householdID, actorID bson.ObjectID, notification Notification) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	devices, err := n.devices.DevicesForHousehold(ctx, householdID, actorID)
	if err != nil {
		n.log.Error("load devices for push", "error", err)
		return
	}
	for _, device := range devices {
		if !n.sender.Supports(device.Topic) {
			continue
		}
		err := n.sender.Send(ctx, device.Token, device.Topic, device.Environment, notification)
		switch {
		case errors.Is(err, ErrUnregistered):
			n.log.Info("forget invalid device token", "reason", err)
			if err := n.devices.DeleteDeviceByToken(ctx, device.Token); err != nil {
				n.log.Error("delete device", "error", err)
			}
		case err != nil:
			n.log.Warn("push failed", "error", err)
		}
	}
}
