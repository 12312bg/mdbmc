package auth

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/12312bg/mdbmc/internal/vault"
)

var (
	ErrNotLoggedIn        = errors.New("未登录或会话已锁定，请重新登录")
	ErrForbidden          = errors.New("当前角色没有执行此操作的权限")
	ErrMustChangePassword = errors.New("请先修改初始密码")
)

// Session is the logged-in user together with the unlocked vault.
type Session struct {
	UserID      int64
	Username    string
	DisplayName string
	Role        Role
	LoginAt     time.Time

	mu         sync.Mutex
	dek        []byte
	closed     bool
	mustChange bool
	box        *vault.Box
}

// Box returns the vault box that seals and opens stored secrets.
func (s *Session) Box() *vault.Box { return s.box }

// DataKey returns a copy of the DEK, needed only to wrap it for another
// password. It fails once the session has ended and the key was wiped.
func (s *Session) DataKey() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrNotLoggedIn
	}
	return append([]byte(nil), s.dek...), nil
}

// MustChangePassword reports whether the user still has to replace an
// initial or reset password before doing anything else.
func (s *Session) MustChangePassword() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mustChange
}

// PasswordChanged lifts the must-change restriction.
func (s *Session) PasswordChanged() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mustChange = false
}

func (s *Session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault.Zero(s.dek)
	s.closed = true
}

// Can reports whether the session's role grants p.
func (s *Session) Can(p Permission) bool { return s.Role.Can(p) }

// NewSession builds a session from an unwrapped data key. With mustChange
// set, Require refuses everything until PasswordChanged is called.
func NewSession(userID int64, username, displayName string, role Role, dek []byte, mustChange bool) (*Session, error) {
	box, err := vault.NewBox(dek)
	if err != nil {
		return nil, err
	}
	return &Session{
		UserID: userID, Username: username, DisplayName: displayName, Role: role,
		LoginAt: time.Now(), dek: dek, box: box, mustChange: mustChange,
	}, nil
}

// Manager holds the single active session of this desktop process and
// throttles repeated login failures.
type Manager struct {
	mu       sync.RWMutex
	current  *Session
	failures map[string]*failure
}

type failure struct {
	count int
	last  time.Time
}

const (
	maxFailures  = 5
	lockoutDelay = 30 * time.Second
)

func NewManager() *Manager {
	return &Manager{failures: map[string]*failure{}}
}

// Current returns the active session or nil.
func (m *Manager) Current() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Set replaces the active session.
func (m *Manager) Set(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clearLocked()
	m.current = s
}

// Clear ends the active session and wipes the data key from memory.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clearLocked()
}

func (m *Manager) clearLocked() {
	if m.current != nil {
		m.current.close()
		m.current = nil
	}
}

// Require returns the active session if it grants every permission in perms
// and the user is not still on an initial password.
func (m *Manager) Require(perms ...Permission) (*Session, error) {
	s := m.Current()
	if s == nil {
		return nil, ErrNotLoggedIn
	}
	if s.MustChangePassword() {
		return nil, ErrMustChangePassword
	}
	for _, p := range perms {
		if !s.Can(p) {
			return nil, ErrForbidden
		}
	}
	return s, nil
}

// CheckThrottle returns an error while a username is locked out after too
// many failed attempts.
func (m *Manager) CheckThrottle(username string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f := m.failures[strings.ToLower(username)]
	if f == nil || f.count < maxFailures {
		return nil
	}
	if wait := lockoutDelay - time.Since(f.last); wait > 0 {
		return fmt.Errorf("密码错误次数过多，请 %d 秒后再试", int(wait.Seconds())+1)
	}
	return nil
}

// RecordFailure counts a failed login for username.
func (m *Manager) RecordFailure(username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(username)
	f := m.failures[key]
	if f == nil {
		f = &failure{}
		m.failures[key] = f
	}
	if f.count >= maxFailures && time.Since(f.last) >= lockoutDelay {
		f.count = 0
	}
	f.count++
	f.last = time.Now()
}

// RecordSuccess resets the failure counter for username.
func (m *Manager) RecordSuccess(username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.failures, strings.ToLower(username))
}
