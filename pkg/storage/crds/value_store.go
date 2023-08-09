package crds

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/golang/snappy"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
}

type CRDValueStoreOptions struct {
	client client.Client
}

type CRDValueStoreOption func(*CRDValueStoreOptions)

func (o *CRDValueStoreOptions) apply(opts ...CRDValueStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithClient(client client.Client) CRDValueStoreOption {
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

	return &CRDValueStore[O, T]{
		objectRef:            objectRef,
		methods:              methods,
		CRDValueStoreOptions: options,
	}
}

func (s *CRDValueStore[O, T]) newEmptyObject() O {
	var obj O // nil pointer of type O
	obj = reflect.New(reflect.TypeOf(obj).Elem()).Interface().(O)
	obj.SetName(s.objectRef.Name)
	obj.SetNamespace(s.objectRef.Namespace)
	return obj
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
	}

	revisionNumber, _ := strconv.ParseInt(obj.GetResourceVersion(), 10, 64)
	driverutil.UnsetRevision(conf)

	if getOpts.RevisionOut != nil {
		*getOpts.RevisionOut = revisionNumber
	}
	return conf, nil

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
