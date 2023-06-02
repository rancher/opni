package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/samber/lo"
	"go.uber.org/multierr"
)

func AtMostOne[Resp any](ctx context.Context, client *http.Client, reqs []*http.Request) (*Resp, error) {
	var retErr error
	for _, req := range reqs {
		var workingResp Resp
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			retErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			retErr = fmt.Errorf("unexpected status code %d", resp.StatusCode)
			continue
		}
		if err := json.NewDecoder(resp.Body).Decode(&workingResp); err != nil {
			retErr = err
			continue
		}
		return &workingResp, nil
	}
	return nil, retErr
}

func AtLeastOne[Resp any](ctx context.Context, client *http.Client, reqs []*http.Request) (*Resp, error) {
	ch := make(chan lo.Tuple2[*Resp, error], len(reqs))
	stopCh := make(chan struct{})
	defer close(ch)
	defer close(stopCh)

	sendOrStop := func(r *Resp, err error) {
		select {
		case <-stopCh:
			return
		default:
			ch <- lo.Tuple2[*Resp, error]{A: r, B: err}
		}
	}
	for _, req := range reqs {
		req := req
		req = req.WithContext(ctx)
		go func() {
			var r Resp
			resp, err := client.Do(req)
			if err != nil {
				sendOrStop(nil, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				sendOrStop(nil, fmt.Errorf("unexpected status code %d", resp.StatusCode))
			}
			if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
				sendOrStop(nil, err)
			}
			sendOrStop(&r, nil)
		}()
	}
	for {
		select {
		case <-ctx.Done():
			stopCh <- struct{}{}
			return nil, ctx.Err()
		case resp := <-ch:
			stopCh <- struct{}{}
			return resp.A, resp.B
		}
	}
}

func MergePartial[Resp any](
	ctx context.Context,
	client *http.Client,
	reqs []*http.Request,
	mergeFn func(cum Resp, next Resp) (out Resp),
) (Resp, error) {
	var agg Resp
	retErrs := []error{}
	ch := make(chan lo.Tuple2[Resp, error], len(reqs))
	numSuccess := 0
	go func() {
		defer close(ch)

		var wg sync.WaitGroup
		wg.Add(len(reqs))
		for _, req := range reqs {
			req := req
			req = req.WithContext(ctx)
			go func() {
				defer wg.Done()
				var r Resp
				resp, err := client.Do(req)
				if err != nil {
					ch <- lo.Tuple2[Resp, error]{A: r, B: err}
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					ch <- lo.Tuple2[Resp, error]{A: r, B: fmt.Errorf("unexpected status code %d", resp.StatusCode)}
					return
				}
				if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
					ch <- lo.Tuple2[Resp, error]{A: r, B: err}
					return
				}
				ch <- lo.Tuple2[Resp, error]{A: r, B: nil}
			}()
		}
		wg.Wait()
	}()

	for {
		select {
		case resp, ok := <-ch:
			if resp.B != nil {
				retErrs = append(retErrs, resp.B)
			} else {
				if numSuccess == 0 {
					agg = resp.A
				} else {
					agg = mergeFn(agg, resp.A)
				}
				numSuccess++
			}
			if !ok {
				if numSuccess == 0 {
					return agg, multierr.Combine(retErrs...)
				}
				return agg, nil
			}
		}
	}
}

func MergeStrict[Resp any](
	ctx context.Context,
	client *http.Client,
	reqs []*http.Request,
	mergeFn func(cum *Resp, next *Resp) (out *Resp),
) (*Resp, error) {
	var agg *Resp
	ch := make(chan lo.Tuple2[*Resp, error], len(reqs))
	go func() {
		defer close(ch)

		var wg sync.WaitGroup
		wg.Add(len(reqs))
		for _, req := range reqs {
			req := req
			req = req.WithContext(ctx)
			go func() {
				defer wg.Done()
				var r Resp
				resp, err := client.Do(req)
				if err != nil {
					ch <- lo.Tuple2[*Resp, error]{A: nil, B: err}
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					ch <- lo.Tuple2[*Resp, error]{A: nil, B: fmt.Errorf("unexpected status code %d", resp.StatusCode)}
					return
				}
				if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
					ch <- lo.Tuple2[*Resp, error]{A: nil, B: err}
					return
				}

				ch <- lo.Tuple2[*Resp, error]{A: &r, B: nil}
			}()
		}
		wg.Wait()
	}()

	for {
		select {
		case resp, ok := <-ch:
			if resp.B != nil {
				return nil, resp.B
			}
			if agg == nil {
				agg = resp.A
			} else {
				agg = mergeFn(agg, resp.A)
			}
			if !ok {
				return agg, nil
			}
		}
	}
}
