package a

import "context"

func work() {}

func leaky() {
	go func() { // want `goroutine lifetime unclear`
		for {
			work()
		}
	}()
}

func clean(ctx context.Context, done chan struct{}) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			default:
				work()
			}
		}
	}()
}

func namedFuncLaunch() {
	go work() // not a literal: out of scope for this rule
}
