package lock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type AccessType byte

const (
	NL AccessType = 1 << 0
	CR AccessType = 1 << 1
	CW AccessType = 1 << 2
	PR AccessType = 1 << 3
	PW AccessType = 1 << 4
	EX AccessType = 1 << 5

	NLMASK = NL | CR | CW | PR | PW | EX
	CRMASK = NL | CR | CW | PR | PW
	CWMASK = NL | CR | CW
	PRMASK = NL | CR | PR
	PWMASK = NL | CR
	EXMASK = NL
)

var (
	BitMasks = map[AccessType]AccessType{
		NL: NLMASK,
		CR: CRMASK,
		CW: CWMASK,
		PR: PRMASK,
		PW: PWMASK,
		EX: EXMASK,
	}
)

var (
	ErrAcquireLockTimeout        = errors.New("acquiring lock error : timeout")
	ErrAcquireLockRetryExceeded  = errors.New("acquiring lock error : retry limit exceeded")
	ErrAcquireLockCancelled      = errors.New("acquiring lock error : context cancelled")
	ErrAcquireLockConflict       = errors.New("acquiring lock error : request has conflicting lock value")
	ErrLockNotFound              = errors.New("lock not found")
	ErrAcquireUnlockTimeout      = errors.New("acquiring unlock error : timeout")
	ErrAcquireUnlockCancelled    = errors.New("acquiring unlock error : cancelled")
	ErrAcquireUnockRetryExceeded = errors.New("acquiring unlock error : retry limit exceeded")
	ErrLockNotAcquired           = errors.New("unlock failed: lock not acquired")

	ErrLockActionRequested = errors.New("lock action already requested")
)

var (
	DefaultMaxRetries     = 10
	DefaultRetryDelay     = 10 * time.Millisecond
	DefaultAcquireTimeout = 100 * time.Millisecond
	DefaultTimeout        = 10 * time.Second
)

func AccessTypeToString(a AccessType) string {
	switch a {
	case NL:
		return "NL"
	case CR:
		return "CR"
	case CW:
		return "CW"
	case PR:
		return "PR"
	case PW:
		return "PW"
	case EX:
		return "EX"
	}
	return "UK"
}

// Modified sync.Once primitive
type LockPrimitive struct {
	Done uint32
	m    sync.Mutex
}

func (l *LockPrimitive) Do(f func() error) error {
	if atomic.LoadUint32(&l.Done) == 0 {
		return l.doSlow(f)
	}
	return ErrLockActionRequested
}

func (l *LockPrimitive) doSlow(f func() error) error {
	l.m.Lock()
	defer l.m.Unlock()
	if l.Done == 0 {
		defer atomic.StoreUint32(&l.Done, 1)
		return f()
	}
	return nil
}

type LockOptions struct {
	MaxRetries     int
	AcquireTimeout time.Duration
	LockValidity   time.Duration
	RetryDelay     time.Duration
	Ctx            context.Context
	AccessType     AccessType
}

func NewLockOptions(ctx context.Context) *LockOptions {
	return &LockOptions{
		MaxRetries:     DefaultMaxRetries,
		RetryDelay:     DefaultRetryDelay,
		AcquireTimeout: DefaultAcquireTimeout,
		LockValidity:   DefaultTimeout,
		Ctx:            ctx,
		AccessType:     EX,
	}
}

func (o *LockOptions) Apply(opts ...LockOption) {
	for _, op := range opts {
		op(o)
	}
}

type LockOption func(o *LockOptions)

func WithMaxRetries(retries int) LockOption {
	return func(o *LockOptions) {
		o.MaxRetries = retries
	}
}

func WithRetryDelay(delay time.Duration) LockOption {
	return func(o *LockOptions) {
		o.RetryDelay = delay
	}
}

func WithAcquireTimeout(timeout time.Duration) LockOption {
	return func(o *LockOptions) {
		o.AcquireTimeout = timeout
	}
}

func WithExpireDuration(expireDuration time.Duration) LockOption {
	return func(o *LockOptions) {
		o.LockValidity = expireDuration
	}
}

func WithContext(ctx context.Context) LockOption {
	return func(o *LockOptions) {
		o.Ctx = ctx
	}
}

func WithExclusiveLock() LockOption {
	return func(o *LockOptions) {
		o.AccessType = EX
	}
}

func WithProtectedWrite() LockOption {
	return func(o *LockOptions) {
		o.AccessType = PW
	}
}

func WithProtectedRead() LockOption {
	return func(o *LockOptions) {
		o.AccessType = PR
	}
}

func WithConcurrentRead() LockOption {
	return func(o *LockOptions) {
		o.AccessType = CR
	}
}

func WithConcurrentWrite() LockOption {
	return func(o *LockOptions) {
		o.AccessType = CW
	}
}

func WithAccessType(at AccessType) LockOption {
	return func(o *LockOptions) {
		o.AccessType = at
	}
}

func Compat(req, existing AccessType) bool {
	return (req & BitMasks[existing]) == req
}
