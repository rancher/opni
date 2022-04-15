package cliutil

import (
	"context"
	"time"

	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StatusObject interface {
	client.Object
	GetState() string
	GetConditions() []string
}

func WaitAndDisplayStatus(
	ctx context.Context,
	timeout time.Duration,
	k8sClient client.Client,
	obj StatusObject,
) error {
	waitCtx, ca := context.WithTimeout(ctx, timeout)
	defer ca()

	p := mpb.New()

	waitingSpinner := p.AddSpinner(1,
		mpb.AppendDecorators(
			decor.OnComplete(decor.Name(chalk.Bold.TextStyle("Waiting for resource to become ready..."), decor.WCSyncSpaceR),
				chalk.Bold.TextStyle("Done."),
			),
		),
		mpb.BarFillerMiddleware(
			CheckBarFiller(waitCtx, func(c context.Context) bool {
				return waitCtx.Err() == nil
			})),
		mpb.BarWidth(1),
	)
	conds := map[string]*mpb.Bar{}

	go func() {
		<-waitCtx.Done()
		waitingSpinner.Increment()
	}()
	wait.PollImmediateUntil(500*time.Millisecond, func() (done bool, err error) {
		err = k8sClient.Get(waitCtx, client.ObjectKeyFromObject(obj), obj)
		if client.IgnoreNotFound(err) != nil {
			return false, err
		}
		state := obj.GetState()
		conditions := obj.GetConditions()

		if state == "Ready" {
			waitingSpinner.Increment()
			done = true
			for _, v := range conds {
				v.Increment()
			}
		}

		for _, cond := range conditions {
			if _, ok := conds[cond]; !ok {
				conds[cond] = p.AddSpinner(1,
					mpb.AppendDecorators(
						func(cond string) decor.Decorator {
							done := false
							var doneText string
							return decor.Any(func(s decor.Statistics) string {
								if done {
									return doneText
								}
								if s.Completed || waitCtx.Err() != nil {
									done = true
									if waitCtx.Err() == nil {
										doneText = chalk.Bold.TextStyle(chalk.Green.Color("[Done] ")) + chalk.Italic.TextStyle(cond)
									} else {
										doneText = chalk.Bold.TextStyle(chalk.Red.Color("[Timed Out] ")) + chalk.Italic.TextStyle(cond)
									}
									return doneText
								}
								return chalk.Bold.TextStyle(chalk.Blue.Color(cond))
							}, decor.WCSyncSpaceR)
						}(cond),
					),
					mpb.BarFillerMiddleware(
						CheckBarFiller(waitCtx, func(c context.Context) bool {
							return waitCtx.Err() == nil
						}),
					),
					mpb.BarWidth(1),
				)
				go func(cond string) {
					<-waitCtx.Done()
					if !conds[cond].Completed() {
						conds[cond].Increment()
					}
				}(cond)
			}
		}

		for k, v := range conds {
			found := false
			for _, cond := range conditions {
				if k == cond {
					found = true
					break
				}
			}
			if !found {
				v.Increment()
			}
		}
		return done, nil
	}, waitCtx.Done())

	p.Wait()
	return nil
}
