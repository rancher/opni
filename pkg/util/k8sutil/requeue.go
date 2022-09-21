package k8sutil

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type RequeueOp struct {
	res ctrl.Result
	err error
}

func LoadResult(result *reconcile.Result, err error) RequeueOp {
	if err != nil {
		return RequeueErr(err)
	}
	if result != nil {
		return RequeueOp{
			res: *result,
		}
	}
	return RequeueOp{}
}

func DoNotRequeue() RequeueOp {
	return RequeueOp{
		res: ctrl.Result{
			Requeue: false,
		},
	}
}

func Requeue() RequeueOp {
	return RequeueOp{
		res: reconcile.Result{
			Requeue: true,
		},
	}
}

func RequeueAfter(d time.Duration) RequeueOp {
	return RequeueOp{
		res: reconcile.Result{
			Requeue:      true,
			RequeueAfter: d,
		},
	}
}

func RequeueErr(err error) RequeueOp {
	return RequeueOp{
		res: reconcile.Result{},
		err: err,
	}
}

func (r RequeueOp) ShouldRequeue() bool {
	return r.res.Requeue || r.res.RequeueAfter > 0 || r.err != nil
}

func (r RequeueOp) Result() (ctrl.Result, error) {
	return r.res, r.err
}

func (r RequeueOp) ResultPtr() (result *ctrl.Result, err error) {
	if !r.res.IsZero() {
		result = &r.res
	}
	err = r.err
	return
}
