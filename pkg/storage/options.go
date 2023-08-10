package storage

import corev1 "github.com/rancher/opni/pkg/apis/core/v1"

type TokenCreateOptions struct {
	Labels       map[string]string
	Capabilities []*corev1.TokenCapability
	MaxUsages    int64
}

func NewTokenCreateOptions() TokenCreateOptions {
	return TokenCreateOptions{
		Labels:       map[string]string{},
		Capabilities: []*corev1.TokenCapability{},
	}
}

type TokenCreateOption func(*TokenCreateOptions)

func (o *TokenCreateOptions) Apply(opts ...TokenCreateOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLabels(labels map[string]string) TokenCreateOption {
	return func(o *TokenCreateOptions) {
		o.Labels = labels
	}
}

func WithCapabilities(capabilities []*corev1.TokenCapability) TokenCreateOption {
	return func(o *TokenCreateOptions) {
		o.Capabilities = capabilities
	}
}

func WithMaxUsages(usages int64) TokenCreateOption {
	return func(o *TokenCreateOptions) {
		o.MaxUsages = usages
	}
}

type AlertFilterOptions struct {
	Labels map[string]string
	Range  *corev1.TimeRange
}

// ================
// KV Store Options
// ================

type GetOptions struct {
	// Get the value at a specific Revision
	Revision *int64

	// If non-nil, will be set to the current revision of the key after the Get
	// operation completes successfully. If an error occurs, no changes
	// will be made to the value.
	RevisionOut *int64
}

type PutOptions struct {
	// Put only if the latest Revision matches
	Revision *int64

	// If non-nil, will be set to the updated revision of the key after the Put
	// operation completes successfully. If an error occurs, no changes
	// will be made to the value.
	RevisionOut *int64
}

type DeleteOptions struct {
	// Delete only if the latest Revision matches
	Revision *int64
}

type ListKeysOptions struct {
	// Maximum number of keys to return
	Limit *int64
}

type HistoryOptions struct {
	// Specifies the latest modification revision to include in the returned
	// history. The history will contain all revisions of the key, starting at
	// the most recent creation revision, and ending at either the specified
	// revision, or the most recent modification revision of the key. If the
	// specified revision is before the latest creation revision, and the
	// key has multiple creation revisions (due to a delete and re-create),
	// then the history will instead start at the most recent creation
	// revision that is <= the specified revision.
	Revision *int64
	// Include the values in the response, not just the metadata. This could
	// have performance implications, so use with caution.
	IncludeValues bool
}

type (
	RevisionOpt      int64
	RevisionOutOpt   struct{ *int64 }
	LimitOpt         int64
	IncludeValuesOpt bool
)

// WithRevision can be used for [GetOptions], [PutOptions], or [DeleteOptions]
func WithRevision(rev int64) RevisionOpt {
	return RevisionOpt(rev)
}

// WithRevisionOut can be used for [GetOptions] or [PutOptions].
func WithRevisionOut(out *int64) RevisionOutOpt {
	return RevisionOutOpt{out}
}

// WithLimit can be used for [ListKeysOptions] or [HistoryOptions].
func WithLimit(limit int64) LimitOpt {
	return LimitOpt(limit)
}

// IncludeValues can be used for [HistoryOptions].
func IncludeValues(include bool) IncludeValuesOpt {
	return IncludeValuesOpt(include)
}

func (r RevisionOpt) ApplyGetOption(opts *GetOptions)         { opts.Revision = (*int64)(&r) }
func (r RevisionOpt) ApplyPutOption(opts *PutOptions)         { opts.Revision = (*int64)(&r) }
func (r RevisionOpt) ApplyDeleteOption(opts *DeleteOptions)   { opts.Revision = (*int64)(&r) }
func (r RevisionOpt) ApplyHistoryOption(opts *HistoryOptions) { opts.Revision = (*int64)(&r) }

func (r RevisionOutOpt) ApplyPutOption(opts *PutOptions) { opts.RevisionOut = r.int64 }
func (r RevisionOutOpt) ApplyGetOption(opts *GetOptions) { opts.RevisionOut = r.int64 }

func (l LimitOpt) ApplyListOption(opts *ListKeysOptions) { opts.Limit = (*int64)(&l) }

func (i IncludeValuesOpt) ApplyHistoryOption(opts *HistoryOptions) { opts.IncludeValues = bool(i) }

type (
	GetOpt     interface{ ApplyGetOption(*GetOptions) }
	PutOpt     interface{ ApplyPutOption(*PutOptions) }
	DeleteOpt  interface{ ApplyDeleteOption(*DeleteOptions) }
	ListOpt    interface{ ApplyListOption(*ListKeysOptions) }
	HistoryOpt interface{ ApplyHistoryOption(*HistoryOptions) }
)

func (o *GetOptions) Apply(opts ...GetOpt) {
	for _, opt := range opts {
		opt.ApplyGetOption(o)
	}
}

func (o *PutOptions) Apply(opts ...PutOpt) {
	for _, opt := range opts {
		opt.ApplyPutOption(o)
	}
}

func (o *DeleteOptions) Apply(opts ...DeleteOpt) {
	for _, opt := range opts {
		opt.ApplyDeleteOption(o)
	}
}

func (o *ListKeysOptions) Apply(opts ...ListOpt) {
	for _, opt := range opts {
		opt.ApplyListOption(o)
	}
}

func (o *HistoryOptions) Apply(opts ...HistoryOpt) {
	for _, opt := range opts {
		opt.ApplyHistoryOption(o)
	}
}
