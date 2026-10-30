package app

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/store"
)

// poller refreshes power and health of all devices on a schedule while a
// user is logged in (the vault key is needed to reach the BMCs).
type poller struct {
	a        *App
	mu       sync.Mutex
	cancel   context.CancelFunc
	kick     chan struct{}
	lastPoll time.Time
	running  bool
}

func newPoller(a *App) *poller {
	return &poller{a: a, kick: make(chan struct{}, 1)}
}

func (p *poller) start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithCancel(p.a.bg())
	p.cancel = cancel
	go p.loop(ctx)
}

func (p *poller) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}

// reschedule wakes the loop so a changed interval takes effect.
func (p *poller) reschedule() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

func (p *poller) state() (int64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var ts int64
	if !p.lastPoll.IsZero() {
		ts = p.lastPoll.Unix()
	}
	return ts, p.running
}

func (p *poller) loop(ctx context.Context) {
	// First pass shortly after login so the UI fills in.
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.kick:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			if sess := p.a.auth.Current(); sess != nil {
				p.pollAll(ctx, sess, nil)
			}
		}
		interval := time.Duration(p.a.Settings().PollIntervalSec) * time.Second
		if interval <= 0 {
			interval = 24 * time.Hour // disabled; wait for a kick
		}
		timer.Reset(interval)
	}
}

// pollNow polls the given devices (all when ids is empty) immediately.
func (p *poller) pollNow(sess *auth.Session, ids []int64) {
	p.pollAll(p.a.bg(), sess, ids)
}

func (p *poller) pollAll(ctx context.Context, sess *auth.Session, ids []int64) {
	var devs []*store.Device
	var err error
	if len(ids) == 0 {
		devs, err = p.a.store.ListDevices(ctx)
	} else {
		devs, err = p.a.store.DevicesByIDs(ctx, ids)
	}
	if err != nil {
		log.Printf("poll: list devices: %v", err)
		return
	}
	full := len(ids) == 0
	if full {
		p.mu.Lock()
		p.running = true
		p.mu.Unlock()
		p.a.emit("poll:state", map[string]any{"running": true})
	}
	var wg sync.WaitGroup
	for _, d := range devs {
		wg.Add(1)
		go func(d *store.Device) {
			defer wg.Done()
			p.pollOne(ctx, sess, d)
		}(d)
	}
	wg.Wait()
	if full {
		p.mu.Lock()
		p.running = false
		p.lastPoll = time.Now()
		ts := p.lastPoll.Unix()
		p.mu.Unlock()
		p.a.emit("poll:state", map[string]any{"running": false, "lastPoll": ts})
	}
}

func (p *poller) pollOne(ctx context.Context, sess *auth.Session, d *store.Device) {
	if ctx.Err() != nil {
		return
	}
	t, err := p.a.target(ctx, sess, d)
	if err != nil {
		return
	}
	release, err := p.a.acquire(ctx, d.ID)
	if err != nil {
		return
	}
	// Start the clock only once it is this device's turn.
	set := p.a.Settings()
	opCtx, cancel := context.WithTimeout(ctx, time.Duration(set.IPMITimeoutSec*3+set.RedfishTimeoutSec*2)*time.Second)
	defer cancel()
	c := bmc.New(t)
	res := c.Status(opCtx)
	c.Close()
	release()
	if ctx.Err() != nil || p.a.auth.Current() != sess {
		return // logged out mid-poll; do not record a spurious failure
	}
	// A failed poll only means offline when the BMC does not answer on the
	// network either (not, say, after a password change).
	if res.Reachable {
		p.a.alerts.observe(d, true, string(res.Power), string(res.Health))
	} else {
		p.a.alerts.observe(d, p.a.alerts.probe(ctx, d), "", "")
	}
	p.a.pinIfFirst(d, c.ObservedFingerprint)
	st := store.Status{
		PowerState: string(res.Power), Health: string(res.Health), Reachable: res.Reachable,
		LastError: res.Error, CheckedAt: time.Now().Unix(),
	}
	if !res.Reachable {
		// Keep the last known power state; it is shown greyed out.
		st.PowerState, st.Health = d.PowerState, string(bmc.HealthUnknown)
	}
	if err := p.a.store.UpdateDeviceStatus(context.Background(), d.ID, st); err != nil {
		log.Printf("poll: save status: %v", err)
	}
	p.a.emitStatus(d.ID, st)
	// Fill in identity for devices that were added while unreachable.
	if res.Reachable && d.IdentifiedAt == 0 {
		go func() { _ = p.a.identifyOne(p.a.bg(), sess, d.ID) }()
	}
}
