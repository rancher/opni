package crds

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/golang/snappy"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ValueStoreMethods[O client.Object, T driverutil.ConfigType[T]] interface {
	ControllerReference() (client.Object, bool)
	FillObjectFromConfig(obj O, conf T)
	FillConfigFromObject(obj O, conf T)
}

type CRDValueStore[O client.Object, T driverutil.ConfigType[T]] struct {
	CRDValueStoreOptions
	objectRef client.ObjectKey
	methods   ValueStoreMethods[O, T]
	gvk       schema.GroupVersionKind
	listType  reflect.Type
}

type CRDValueStoreOptions struct {
	client client.WithWatch
}

type CRDValueStoreOption func(*CRDValueStoreOptions)

func (o *CRDValueStoreOptions) apply(opts ...CRDValueStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithClient(client client.WithWatch) CRDValueStoreOption {
	return func(o *CRDValueStoreOptions) {
		o.client = client
	}
}

func NewCRDValueStore[O client.Object, T driverutil.ConfigType[T]](
	objectRef client.ObjectKey,
	methods ValueStoreMethods[O, T],
	opts ...CRDValueStoreOption,
) storage.ValueStoreT[T] {
	options := CRDValueStoreOptions{}
	options.apply(opts...)

	if options.client == nil {
		var err error
		options.client, err = k8sutil.NewK8sClient(k8sutil.ClientOptions{})
		if err != nil {
			panic(err)
		}
	}

	var obj O
	obj = reflect.New(reflect.TypeOf(obj).Elem()).Interface().(O)
	gvks, _, err := options.client.Scheme().ObjectKinds(obj)
	if err != nil {
		panic(fmt.Sprintf("bug: failed to get object kind for %T: %v", obj, err))
	}
	if len(gvks) == 0 {
		panic(fmt.Sprintf("bug: no object kind for %T available in scheme", obj))
	}
	if len(gvks) > 1 {
		panic(fmt.Sprintf("bug: %T has multiple object kinds in scheme: %v", obj, gvks))
	}
	// look up the associated list type
	types := options.client.Scheme().KnownTypes(gvks[0].GroupVersion())
	listType, ok := types[gvks[0].Kind+"List"]
	if !ok {
		panic(fmt.Sprintf("bug: no list type for %T available in scheme", obj))
	}

	return &CRDValueStore[O, T]{
		objectRef:            objectRef,
		methods:              methods,
		CRDValueStoreOptions: options,
		gvk:                  gvks[0],
		listType:             listType,
	}
}

func (s *CRDValueStore[O, T]) newEmptyObject() O {
	var obj O // nil pointer of type O
	obj = reflect.New(reflect.TypeOf(obj).Elem()).Interface().(O)
	obj.GetObjectKind().SetGroupVersionKind(s.gvk)
	obj.SetName(s.objectRef.Name)
	obj.SetNamespace(s.objectRef.Namespace)
	return obj
}

func (s *CRDValueStore[O, T]) newEmptyObjectList() client.ObjectList {
	objList := reflect.New(s.listType).Interface().(client.ObjectList)
	objList.GetObjectKind().SetGroupVersionKind(s.gvk.GroupVersion().WithKind(s.gvk.Kind + "List"))
	return objList
}

func (s *CRDValueStore[O, T]) newEmptyConfig() T {
	var conf T // nil pointer of type T
	return conf.ProtoReflect().New().Interface().(T)
}

func (s *CRDValueStore[O, T]) Put(ctx context.Context, value T, opts ...storage.PutOpt) error {
	// special case: if value is nil, it needs to be set to the corresponding
	// empty object due to the way nil messages are handled
	if reflect.ValueOf(value).IsNil() {
		value = s.newEmptyConfig()
	}

	putOptions := storage.PutOptions{}
	putOptions.Apply(opts...)

	obj := s.newEmptyObject()
	err := s.client.Get(ctx, s.objectRef, obj)
	exists := true
	if err != nil {
		if k8serrors.IsNotFound(err) {
			exists = false
		} else {
			return toGrpcError(err)
		}
	}

	if exists {
		s.appendSpecToHistory(obj)
	}

	if ref, ok := s.methods.ControllerReference(); ok {
		controllerutil.SetControllerReference(ref, obj, s.client.Scheme())
	}

	s.methods.FillObjectFromConfig(obj, value)

	if !exists {
		err := s.client.Create(ctx, obj)
		if err != nil {
			return toGrpcError(err)
		}
	} else {
		if putOptions.Revision != nil {
			if *putOptions.Revision == 0 {
				return fmt.Errorf("%w: expected object not to exist (requested revision 0)", storage.ErrConflict)
			}
			obj.SetResourceVersion(strconv.FormatInt(*putOptions.Revision, 10))
		}

		err := s.client.Update(ctx, obj)
		if err != nil {
			return toGrpcError(err)
		}
	}
	if putOptions.RevisionOut != nil {
		*putOptions.RevisionOut, _ = strconv.ParseInt(obj.GetResourceVersion(), 10, 64)
	}

	return nil
}

func (s *CRDValueStore[O, T]) Get(ctx context.Context, opts ...storage.GetOpt) (T, error) {
	getOpts := storage.GetOptions{}
	getOpts.Apply(opts...)

	obj := s.newEmptyObject()
	err := s.client.Get(ctx, s.objectRef, obj)
	var zero T
	if err != nil {
		return zero, toGrpcError(err)
	}

	latestRevision, _ := strconv.ParseInt(obj.GetResourceVersion(), 10, 64)

	var conf T
	var confRevision int64
	if getOpts.Revision != nil && *getOpts.Revision != latestRevision {
		var history historyFormat[T]
		if str, ok := obj.GetAnnotations()["opni.io/history"]; ok {
			// not using readHistory here because the previous check for the
			// latest revision is quicker, and we aren't returning the history
			decodeHistory(str, &history)
		}
		found := false
		for _, entry := range history.Entries {
			rev := entry.Config.GetRevision().GetRevision()
			if rev == *getOpts.Revision {
				conf = entry.Config
				confRevision = rev
				found = true
				break
			}
		}
		if !found {
			if *getOpts.Revision > latestRevision {
				return zero, status.Errorf(codes.OutOfRange, "revision %d is a future revision", *getOpts.Revision)
			}
			return zero, status.Errorf(codes.NotFound, "revision %d not found", *getOpts.Revision)
		}
	} else {
		conf = s.newEmptyConfig()
		s.methods.FillConfigFromObject(obj, conf)
		confRevision, _ = strconv.ParseInt(obj.GetResourceVersion(), 10, 64)
	}

	driverutil.UnsetRevision(conf)
	if getOpts.RevisionOut != nil {
		*getOpts.RevisionOut = confRevision
	}
	return conf, nil
}

func (s *CRDValueStore[O, T]) Watch(ctx context.Context, opts ...storage.WatchOpt) (<-chan storage.WatchEvent[storage.KeyRevision[T]], error) {
	watchOpts := storage.WatchOptions{}
	watchOpts.Apply(opts...)
	if watchOpts.Prefix {
		return nil, status.Errorf(codes.Unimplemented, "prefix watches are not supported by this storage backend")
	}

	objList := s.newEmptyObjectList()

	// https://kubernetes.io/docs/reference/using-api/api-concepts/#the-resourceversion-parameter
	listOpts := &client.ListOptions{
		Namespace:     s.objectRef.Namespace,
		FieldSelector: fields.OneTermEqualSelector("metadata.name", s.objectRef.Name),
	}
	var latestVersion string
	{
		obj := s.newEmptyObject()
		s.client.Get(ctx, s.objectRef, obj) // ok if this fails, version will be empty
		latestVersion = obj.GetResourceVersion()
	}
	watcher, err := s.client.Watch(ctx, objList, listOpts)
	if err != nil {
		return nil, toGrpcError(err)
	}

	eventC := make(chan storage.WatchEvent[storage.KeyRevision[T]], 64)
	go func() {
		defer func() {
			watcher.Stop()
			close(eventC)
		}()

		rc := watcher.ResultChan()
		var previous *storage.KeyRevisionImpl[T]
	INITIAL:
		select {
		case <-ctx.Done():
			return
		case res, ok := <-rc:
			if !ok {
				return
			}
			var eventType storage.WatchEventType
			switch res.Type {
			case watch.Added, watch.Modified:
				eventType = storage.WatchEventPut
			case watch.Deleted:
				eventType = storage.WatchEventDelete
			default:
				break INITIAL
			}
			obj := res.Object.DeepCopyObject().(O)
			currentRevision, _ := strconv.ParseInt(obj.GetResourceVersion(), 10, 64)

			if watchOpts.Revision != nil {
				var history historyFormat[T]
				s.readHistory(obj, &history)
				for _, entry := range history.Entries {
					rev := entry.Config.GetRevision()
					if rev.GetRevision() == currentRevision {
						break
					}
					driverutil.UnsetRevision(entry.Config)
					kr := &storage.KeyRevisionImpl[T]{
						V:    entry.Config,
						Rev:  rev.GetRevision(),
						Time: rev.GetTimestamp().AsTime(),
					}
					if rev.Revision != nil && *rev.Revision >= *watchOpts.Revision && *rev.Revision < currentRevision {
						eventC <- storage.WatchEvent[storage.KeyRevision[T]]{
							EventType: eventType,
							Current:   s.cloneKeyRevision(kr),
							Previous:  s.cloneKeyRevision(previous),
						}
					}
					previous = kr
				}
			}
			// send the current value
			conf := s.newEmptyConfig()
			s.methods.FillConfigFromObject(obj, conf)
			driverutil.UnsetRevision(conf)
			current := &storage.KeyRevisionImpl[T]{
				V:   conf,
				Rev: currentRevision,
			}
			currentEvent := storage.WatchEvent[storage.KeyRevision[T]]{
				EventType: eventType,
				Current:   s.cloneKeyRevision(current),
				Previous:  s.cloneKeyRevision(previous),
			}
			// we only want to send the current revision in the following cases:
			// 1. The revision option is set
			// 2. The revision option is not set, but the object was created after
			//    the watch started
			if watchOpts.Revision != nil || obj.GetResourceVersion() != latestVersion {
				eventC <- currentEvent
			}
			previous = current
		}
		for {
			select {
			case <-ctx.Done():
				return
			case res, ok := <-rc:
				if !ok {
					return
				}
				var ev storage.WatchEvent[storage.KeyRevision[T]]
				obj := res.Object.DeepCopyObject().(O)
				conf := s.newEmptyConfig()
				s.methods.FillConfigFromObject(obj, conf)
				revisionNumber, _ := strconv.ParseInt(obj.GetResourceVersion(), 10, 64)
				driverutil.UnsetRevision(conf)

				switch res.Type {
				case watch.Added, watch.Modified:
					ev.EventType = storage.WatchEventPut
					current := &storage.KeyRevisionImpl[T]{
						Rev: revisionNumber,
						V:   conf,
					}
					ev.Current = s.cloneKeyRevision(current)
					if previous != nil {
						ev.Previous = s.cloneKeyRevision(previous)
					}
					previous = current
				case watch.Deleted:
					ev.EventType = storage.WatchEventDelete
					var previousRevision int64
					if previous == nil {
						previousRevision = s.getPreviousRevisionSlow(obj)
						continue
					} else {
						previousRevision = previous.Revision()
					}
					ev.Previous = &storage.KeyRevisionImpl[T]{
						Rev: previousRevision,
						V:   util.ProtoClone(conf),
					}
					previous = nil
				default:
					continue
				}
				eventC <- ev
			}
		}
	}()

	return eventC, nil
}

func (s *CRDValueStore[O, T]) cloneKeyRevision(kr *storage.KeyRevisionImpl[T]) storage.KeyRevision[T] {
	if kr == nil {
		// it is important that the return value is the interface type, otherwise
		// we would end up with a non-nil interface containing a nil pointer
		return nil
	}
	rev := &storage.KeyRevisionImpl[T]{
		K:    kr.K,
		Rev:  kr.Rev,
		Time: kr.Time,
	}
	if kr.V.ProtoReflect().IsValid() {
		rev.V = util.ProtoClone(kr.V)
	}
	return rev
}

func (s *CRDValueStore[O, T]) getPreviousRevisionSlow(obj O) int64 {
	// The resource revision sent by k8s ends up being the revision of the
	// actual deleted object. For API consistency, we want to return the
	// revision of the last put/create operation on the object. This info
	// will be stored in the history.
	var history historyFormat[T]
	if str, ok := obj.GetAnnotations()["opni.io/history"]; ok {
		decodeHistory(str, &history)
	}
	if len(history.Entries) == 0 {
		// something went wrong, just return the deleted revision
		// (it should still work for most operations)
		rev, _ := strconv.ParseInt(obj.GetResourceVersion(), 10, 64)
		return rev
	}
	return history.Entries[len(history.Entries)-1].Config.GetRevision().GetRevision()
}

func (s *CRDValueStore[O, T]) Delete(ctx context.Context, opts ...storage.DeleteOpt) error {
	deleteOpts := storage.DeleteOptions{}
	deleteOpts.Apply(opts...)

	obj := s.newEmptyObject()
	err := s.client.Get(ctx, s.objectRef, obj)
	if err != nil {
		return toGrpcError(err)
	}

	var clientOpts []client.DeleteOption
	if deleteOpts.Revision != nil {
		clientOpts = append(clientOpts, client.Preconditions{
			ResourceVersion: lo.ToPtr(strconv.FormatInt(*deleteOpts.Revision, 10)),
		})
	}

	err = s.client.Delete(ctx, obj, clientOpts...)
	if err != nil {
		return toGrpcError(err)
	}
	return nil
}

func (s *CRDValueStore[O, T]) History(ctx context.Context, opts ...storage.HistoryOpt) ([]storage.KeyRevision[T], error) {
	historyOpts := storage.HistoryOptions{}
	historyOpts.Apply(opts...)

	var history historyFormat[T]
	obj := s.newEmptyObject()
	err := s.client.Get(ctx, s.objectRef, obj)
	if err != nil {
		if k8serrors.IsNotFound(err) && historyOpts.Revision != nil {
			return nil, status.Errorf(codes.Unimplemented, "this storage backend does not support tracking history for deleted objects")
		}
		return nil, toGrpcError(err)
	}

	s.readHistory(obj, &history)

	entries := make([]storage.KeyRevision[T], 0, len(history.Entries))
	for _, entry := range history.Entries {
		revision := entry.Config.GetRevision()
		driverutil.UnsetRevision(entry.Config)
		rev := &storage.KeyRevisionImpl[T]{
			Rev:  revision.GetRevision(),
			Time: revision.GetTimestamp().AsTime(),
		}
		if historyOpts.IncludeValues {
			rev.V = entry.Config
		}
		entries = append(entries, rev)
		if historyOpts.Revision != nil && rev.Rev == *historyOpts.Revision {
			break
		}
	}
	return entries, nil
}

type historyFormat[T driverutil.ConfigType[T]] struct {
	Entries []historyEntry[T] `json:"entries"`
}

type historyEntry[T driverutil.ConfigType[T]] struct {
	Wire   []byte `json:"wire"`
	Config T      `json:"-"`
}

func (e *historyEntry[T]) MarshalJSON() ([]byte, error) {
	if reflect.ValueOf(e.Config).IsNil() {
		return nil, nil
	}
	var err error
	e.Wire, err = proto.Marshal(e.Config)
	if err != nil {
		return nil, err
	}
	type entry historyEntry[T]
	return json.Marshal((*entry)(e))
}

func (e *historyEntry[T]) UnmarshalJSON(data []byte) error {
	type entry historyEntry[T]
	err := json.Unmarshal(data, (*entry)(e))
	if err != nil {
		return err
	}
	if len(e.Wire) == 0 {
		return nil
	}
	if reflect.ValueOf(e.Config).IsZero() {
		e.Config = e.Config.ProtoReflect().New().Interface().(T)
	}
	return proto.Unmarshal(e.Wire, e.Config)
}

func (s *CRDValueStore[O, T]) appendSpecToHistory(obj O) error {
	conf := s.newEmptyConfig()
	s.methods.FillConfigFromObject(obj, conf)

	var history historyFormat[T]
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	if str, ok := annotations["opni.io/history"]; ok {
		decodeHistory(str, &history)
	} else {
		history.Entries = []historyEntry[T]{}
	}
	var lastModifiedTime time.Time

	if lastModified, ok := annotations["opni.io/last-modified"]; ok {
		lastModifiedTime, _ = time.Parse(time.RFC3339Nano, lastModified)
	} else {
		lastModifiedTime = obj.GetCreationTimestamp().Time
	}
	annotations["opni.io/last-modified"] = time.Now().Format(time.RFC3339Nano)
	if revisionNumber, err := strconv.ParseInt(obj.GetResourceVersion(), 10, 64); err == nil {
		driverutil.SetRevision(conf, revisionNumber, lastModifiedTime)
	}
	history.Entries = append(history.Entries, historyEntry[T]{
		Config: conf,
	})
	// keep only the newest 64 entries
	if len(history.Entries) > 64 {
		history.Entries = history.Entries[len(history.Entries)-64:]
	}

	var err error
	annotations["opni.io/history"], err = encodeHistory(&history)
	if err != nil {
		return err
	}

	obj.SetAnnotations(annotations)
	return nil
}

func (s *CRDValueStore[O, T]) readHistory(obj O, hist *historyFormat[T]) error {
	annotations := obj.GetAnnotations()
	if str, ok := annotations["opni.io/history"]; ok {
		decodeHistory(str, hist)
	} else {
		hist.Entries = []historyEntry[T]{}
	}
	conf := s.newEmptyConfig()
	s.methods.FillConfigFromObject(obj, conf)

	// fill in the revision of the current object
	revisionNumber, _ := strconv.ParseInt(obj.GetResourceVersion(), 10, 64)
	var lastModifiedTime time.Time
	if lastModified, ok := annotations["opni.io/last-modified"]; ok {
		lastModifiedTime, _ = time.Parse(time.RFC3339Nano, lastModified)
	} else {
		lastModifiedTime = obj.GetCreationTimestamp().Time
	}
	driverutil.SetRevision(conf, revisionNumber, lastModifiedTime)

	hist.Entries = append(hist.Entries, historyEntry[T]{
		Config: conf,
	})
	return nil
}

func decodeHistory[T driverutil.ConfigType[T]](str string, hist *historyFormat[T]) error {
	b64, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return err
	}
	decodeLen, err := snappy.DecodedLen(b64)
	if err != nil {
		return err
	}
	out := make([]byte, 0, decodeLen)
	jsonData, err := snappy.Decode(out, b64)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(jsonData, hist); err != nil {
		return err
	}

	for i := range hist.Entries {
		if err := proto.Unmarshal(hist.Entries[i].Wire, hist.Entries[i].Config); err != nil {
			return err
		}
	}
	return nil
}

func encodeHistory[T driverutil.ConfigType[T]](history *historyFormat[T]) (string, error) {
	for i := range history.Entries {
		opts := proto.MarshalOptions{}
		var err error
		if history.Entries[i].Wire, err = opts.MarshalAppend(history.Entries[i].Wire[:0], history.Entries[i].Config); err != nil {
			return "", err
		}
	}
	jsonData, err := json.Marshal(history)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 0, snappy.MaxEncodedLen(len(jsonData)))
	return base64.StdEncoding.EncodeToString(snappy.Encode(buf, jsonData)), nil
}

func toGrpcError(err error) error {
	if err == nil {
		return nil
	}
	if k8serrors.IsNotFound(err) {
		return status.Errorf(codes.NotFound, err.Error())
	}
	if k8serrors.IsConflict(err) {
		return status.Errorf(codes.Aborted, err.Error())
	}
	if k8serrors.IsAlreadyExists(err) {
		return status.Errorf(codes.AlreadyExists, err.Error())
	}
	if k8serrors.IsInvalid(err) || k8serrors.IsBadRequest(err) {
		return status.Errorf(codes.InvalidArgument, err.Error())
	}
	return err
}
